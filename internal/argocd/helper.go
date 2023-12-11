package argocd

import (
	"context"
	"fmt"
	"strings"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/rs/zerolog/log"
)

// Called by processEvent() in main.go to fetch matching ArgoCD applications (based on repo owner & name)
// and return any changes that are contained at the commit sha in revision
func GetApplicationChanges(ctx context.Context, repoOwner, repoName, repoDefaultRef, revision, changeRef, baseRef string) ([]ApplicationResourcesWithChanges, error) {
	var appResList []ApplicationResourcesWithChanges
	argoApps, err := listApplications(ctx)
	if err != nil {
		return appResList, err
	}
	if len(argoApps.Items) == 0 {
		return appResList, fmt.Errorf("empty ArgoCD app list")
	}
	apps, err := filterApplications(argoApps.Items, repoOwner, repoName, repoDefaultRef, changeRef, baseRef)
	if err != nil {
		return appResList, err
	}
	log.Debug().Msgf("Matching apps: %v", apps)

	for _, app := range apps {
		applicationResourceChanges, _ := GetApplicationDiff(ctx, app.ObjectMeta.Name, app.ObjectMeta.Namespace, revision)
		appResList = append(appResList, applicationResourceChanges)
	}
	return appResList, nil
}

// Called by processEvent() in main.go to fetch matching ArgoCD applications (based on repo owner & name)
// and perform a dry-run sync (with a fallback to fetching manifests)
func SyncApplications(ctx context.Context, repoOwner, repoName, repoDefaultRef, revision, changeRef, baseRef string) ([]ApplicationSyncResult, error) {
	var appSyncList []ApplicationSyncResult
	argoApps, err := listApplications(ctx)
	if err != nil {
		return appSyncList, err
	}
	if len(argoApps.Items) == 0 {
		return appSyncList, fmt.Errorf("empty ArgoCD app list")
	}
	apps, err := filterApplications(argoApps.Items, repoOwner, repoName, repoDefaultRef, changeRef, baseRef)
	if err != nil {
		return appSyncList, err
	}
	log.Debug().Msgf("Matching apps: %v", apps)

	for _, app := range apps {
		// get manifests
		_, err := getAppManifests(ctx, app.ObjectMeta.Name, app.ObjectMeta.Namespace, revision)
		if err != nil {
			appSyncList = append(appSyncList, ApplicationSyncResult{
				ManifestsFetchMsg: fmt.Sprintf("Failed to fetch application manifests %s at revision %s: %s", app.ObjectMeta.Name, revision, err.Error()),
				ManifestsFetched:  false,
				SyncSuccess:       SyncSkip,
				SyncMsg:           "Skipped app sync due to manifest fetch failure",
			})
			continue
		}
		appSyncRes := ApplicationSyncResult{
			ArgoApp:           &app,
			ManifestsFetched:  true,
			ManifestsFetchMsg: "Success",
			SyncSuccess:       SyncFail,
			SyncMsg:           "Unknown",
		}
		// get sync options (eg: use server-side apply?)
		dryRun := true
		prune := true
		syncReq := application.ApplicationSyncRequest{
			Name:         &app.ObjectMeta.Name,
			AppNamespace: &app.ObjectMeta.Namespace,
			DryRun:       &dryRun,
			Revision:     &revision,
			Prune:        &prune,
			Strategy:     &v1alpha1.SyncStrategy{Hook: &v1alpha1.SyncStrategyHook{}}, // TODO support apply maybe?
		}
		// pull sync options from app config (if it's set)
		if app.Spec.SyncPolicy != nil && app.Spec.SyncPolicy.Automated != nil {
			*syncReq.Prune = app.Spec.SyncPolicy.Automated.Prune
			if len(app.Spec.SyncPolicy.SyncOptions) > 0 {
				syncReq.SyncOptions = &application.SyncOptions{
					Items: app.Spec.SyncPolicy.SyncOptions,
				}
			}
		}
		_, err = syncApplication(ctx, syncReq)
		if err != nil {
			appSyncRes.SyncSuccess = SyncFail
			appSyncRes.SyncMsg = err.Error()
		}
		appSyncRes.SyncSuccess = SyncSuccess
		appSyncRes.SyncMsg = ""
		appSyncList = append(appSyncList, appSyncRes)
	}
	return appSyncList, nil
}

// Returns a list of Applications whose git URLs match repo owner & name
func filterApplications(a []v1alpha1.Application, repoOwner, repoName, repoDefaultRef, changeRef, baseRef string) ([]v1alpha1.Application, error) {
	var appList []v1alpha1.Application
	ghMatch1 := fmt.Sprintf("github.com/%s/%s.git", repoOwner, repoName)
	ghMatch2 := fmt.Sprintf("github.com/%s/%s", repoOwner, repoName)
	for _, app := range a {
		if !strings.HasSuffix(app.Spec.Source.RepoURL, ghMatch1) && !strings.HasSuffix(app.Spec.Source.RepoURL, ghMatch2) {
			log.Debug().Msgf("Filtering application %s: RepoURL %s doesn't much %s or %s", app.ObjectMeta.Name, app.Spec.Source.RepoURL, ghMatch1, ghMatch2)
			continue
		}
		if baseRef != "" {
			// Processing a PR ...
			if app.Spec.Source.TargetRevision == "HEAD" && baseRef != repoDefaultRef {
				// filter application if argo targets repo default (eg: main) and PR is not targetting main
				log.Debug().Msgf("Filtering application %s: Target Rev is HEAD; baseRef %s != repoDefaultRef %s", app.ObjectMeta.Name, baseRef, repoDefaultRef)
				continue
			}
			if app.Spec.Source.TargetRevision != "HEAD" && baseRef != app.Spec.Source.TargetRevision {
				// filter application if argo doesn't target repo default (eg: main)  and PR is not targetting that branch
				log.Debug().Msgf("Filtering application %s: baseRef %s != Target Rev %s", app.ObjectMeta.Name, baseRef, app.Spec.Source.TargetRevision)
				continue
			}
		} else {
			// processing a push
			// eg: refs/heads/main -> main
			changeRef = strings.TrimPrefix(changeRef, "refs/heads/")
			// filter out apps where auto-sync is enabled for the branch of the push
			if app.Spec.Source.TargetRevision == "HEAD" && changeRef == repoDefaultRef && app.Spec.SyncPolicy != nil && app.Spec.SyncPolicy.Automated != nil {
				log.Debug().Msgf("Filtering auto-sync application %s: Target Rev is HEAD; changeRef %s == repoDefaultRef %s", app.ObjectMeta.Name, changeRef, repoDefaultRef)
				continue
			}
			if app.Spec.Source.TargetRevision != "HEAD" && changeRef == app.Spec.Source.TargetRevision && app.Spec.SyncPolicy != nil && app.Spec.SyncPolicy.Automated != nil {
				log.Debug().Msgf("Filtering auto-sync application %s: changeRef %s = Target Rev %s", app.ObjectMeta.Name, changeRef, app.Spec.Source.TargetRevision)
				continue
			}
		}
		appList = append(appList, app)
	}
	return appList, nil
}
