package argocd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/rs/zerolog/log"
)

const minVersion = "2.12.0"

// checks if a given version is greater than or equal to required version
func versionCheck(version string) bool {

	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")
	minVersionParts := strings.Split(minVersion, ".")
	versionParts := strings.Split(version, ".")

	// Compare major, minor, and patch versions
	for i := 0; i < 3; i++ {
		v1, err1 := strconv.Atoi(versionParts[i])
		v2, err2 := strconv.Atoi(minVersionParts[i])
		// Handle parsing errors
		if err1 != nil || err2 != nil {
			return false
		}
		if v1 > v2 {
			return true
		} else if v1 < v2 {
			return false
		}
	}
	// All parts are equal
	return true
}

func ConnectivityCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	log.Info().Msg("Calling ArgoCD to check client and server versions")
	clientV, serverV, err := argocdVersion(ctx)
	if err != nil {
		return err
	}
	if versionCheck(clientV) && versionCheck(serverV) {
		return nil
	}
	return fmt.Errorf("client (%s) or Server (%s) version is not %s or greater", clientV, serverV, minVersion)
}

// Called by processEvent() in main.go to fetch matching ArgoCD applications (based on repo owner & name)
// and return their manifests.
func GetApplicationChanges(ctx context.Context, repoOwner, repoName, repoDefaultRef, revision, changeRef, baseRef string) ([]ApplicationResourcesWithChanges, error) {
	log.Trace().Msgf("GetApplicationChanges(%s, %s, %s, %s, %s, %s)", repoOwner, repoName, repoDefaultRef, revision, changeRef, baseRef)
	var appResList []ApplicationResourcesWithChanges
	argoApps, err := listApplications(ctx)
	if err != nil {
		return appResList, err
	}
	log.Trace().Msgf("listApplications() returned %d items", len(argoApps.Items))
	if len(argoApps.Items) == 0 {
		return appResList, fmt.Errorf("empty ArgoCD app list")
	}
	apps, err := filterApplications(argoApps.Items, repoOwner, repoName, repoDefaultRef, changeRef, baseRef)
	if err != nil {
		return appResList, err
	}
	log.Debug().Msgf("Matching apps: %s", func() (s string) {
		for _, app := range apps {
			if s != "" {
				s += ", " + app.ObjectMeta.Name
			} else {
				s += app.ObjectMeta.Name
			}
		}
		return
	}())

	for _, app := range apps {
		log.Info().Msgf("Generating application diff for ArgoCD App '%s' w/ revision %s", app.ObjectMeta.Name, revision)
		var appResChanges ApplicationResourcesWithChanges
		//appResChanges.ArgoApp, err = getApplication(ctx, app.ObjectMeta.Name)
		appResChanges.ArgoApp = &app
		if err != nil {
			appResChanges.WarnStr = fmt.Sprintf("Failed to refresh application %s: %s", app.ObjectMeta.Name, err.Error())
		} else {
			appResChanges.ChangedResources, err = diffApplication(ctx, app.ObjectMeta.Name, revision)
			if err != nil {
				appResChanges.WarnStr = fmt.Sprintf("Failed to diff application %s: %s", app.ObjectMeta.Name, err.Error())
			} else {
				if len(appResChanges.ChangedResources) > 0 {
					appResList = append(appResList, appResChanges)
				}
			}
		}
	}
	return appResList, nil
}

// Returns a list of Applications whose git URLs match repo owner & name
func filterApplications(a []v1alpha1.Application, repoOwner, repoName, repoDefaultRef, changeRef, baseRef string) ([]v1alpha1.Application, error) {
	log.Trace().Msgf("filterApplications([%d apps], %s, %s, %s, %s, %s)", len(a), repoOwner, repoName, repoDefaultRef, changeRef, baseRef)
	var appList []v1alpha1.Application
	ghMatch1 := fmt.Sprintf("github.com/%s/%s.git", repoOwner, repoName)
	ghMatch2 := fmt.Sprintf("github.com/%s/%s", repoOwner, repoName)
	log.Debug().Msgf("filterApplications() - matching candidates against '%s' and '%s'", ghMatch1, ghMatch2)
	for _, app := range a {
		if len(app.Spec.Sources) > 0 {
			log.Info().Msgf("Application %s has multiple sources - skipping as it is not supported", app.ObjectMeta.Name)
			continue
		}
		appSpecSource := app.Spec.GetSource()

		if !strings.HasSuffix(appSpecSource.RepoURL, ghMatch1) && !strings.HasSuffix(appSpecSource.RepoURL, ghMatch2) {
			log.Debug().Msgf("Filtering application %s: RepoURL %s doesn't much %s or %s", app.ObjectMeta.Name, appSpecSource.RepoURL, ghMatch1, ghMatch2)
			continue
		}
		if baseRef != "" {
			// Processing a PR ...
			if appSpecSource.TargetRevision == "HEAD" && baseRef != repoDefaultRef {
				// filter application if argo targets repo default (eg: main) and PR is not targetting main
				log.Debug().Msgf("Filtering application %s: Target Rev is HEAD; baseRef %s != repoDefaultRef %s", app.ObjectMeta.Name, baseRef, repoDefaultRef)
				continue
			}
			if appSpecSource.TargetRevision != "HEAD" && baseRef != appSpecSource.TargetRevision {
				// filter application if argo doesn't target repo default (eg: main)  and PR is not targetting that branch
				log.Debug().Msgf("Filtering application %s: baseRef %s != Target Rev %s", app.ObjectMeta.Name, baseRef, appSpecSource.TargetRevision)
				continue
			}
		} else {
			// processing a push
			// eg: refs/heads/main -> main
			changeRef = strings.TrimPrefix(changeRef, "refs/heads/")
			// filter out apps where auto-sync is enabled for the branch of the push
			if appSpecSource.TargetRevision == "HEAD" && changeRef == repoDefaultRef && app.Spec.SyncPolicy != nil && app.Spec.SyncPolicy.Automated != nil {
				log.Debug().Msgf("Filtering auto-sync application %s: Target Rev is HEAD; changeRef %s == repoDefaultRef %s", app.ObjectMeta.Name, changeRef, repoDefaultRef)
				continue
			}
			if appSpecSource.TargetRevision != "HEAD" && changeRef == appSpecSource.TargetRevision && app.Spec.SyncPolicy != nil && app.Spec.SyncPolicy.Automated != nil {
				log.Debug().Msgf("Filtering auto-sync application %s: changeRef %s = Target Rev %s", app.ObjectMeta.Name, changeRef, appSpecSource.TargetRevision)
				continue
			}
		}
		appList = append(appList, app)
	}
	return appList, nil
}
