package argocd

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"sigs.k8s.io/yaml"

	"github.com/vince-riv/argo-diff/internal/webhook"
)

const argoApplicationApiGroup = "argoproj.io"
const argoApplicationApiKind = "Application"
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

func appListToMap(appList []Application) map[string]Application {
	argoAppMap := make(map[string]Application)
	for _, app := range appList {
		argoAppMap[app.ObjectMeta.Name] = app
	}
	return argoAppMap
}

func getApplicationChanges(ctx context.Context, app *Application, revision string, revs []string, pos []int) (ApplicationResourcesWithChanges, error) {
	var appResChanges ApplicationResourcesWithChanges
	var err error
	appResChanges.ArgoApp = app
	if revision != "" {
		appResChanges.ChangedResources, err = diffApplication(ctx, app.ObjectMeta.Name, revision, nil, nil)
	} else {
		if len(revs) < 1 || len(revs) != len(pos) {
			return appResChanges, fmt.Errorf("getApplicationChanges() called as multi-src with bad revs/pos count [%d/%d]", len(revs), len(pos))
		}
		appResChanges.ChangedResources, err = diffApplication(ctx, app.ObjectMeta.Name, "", revs, pos)
	}
	return appResChanges, err
}

func getMultiSrcAppChanges(ctx context.Context, appCur *Application, appNew *Application, repoOwner, repoName, revision string) (ApplicationResourcesWithChanges, error) {
	var appResChanges ApplicationResourcesWithChanges
	appName := appCur.ObjectMeta.Name
	curSources := appCur.Spec.GetSources()
	newSources := appNew.Spec.GetSources()
	if len(curSources) != len(newSources) {
		return appResChanges, fmt.Errorf("number of sources for %s changing: %d -> %d", appName, len(curSources), len(newSources))
	}
	if len(curSources) < 1 {
		return appResChanges, fmt.Errorf("%s has no sources configured", appName)
	}
	revisions := []string{}
	positions := []int{}
	for i, curSrc := range curSources {
		newRevision := newSources[i].TargetRevision
		if curSrc.RepoURL != newSources[i].RepoURL {
			return appResChanges, fmt.Errorf("source URL is changing in %s", appName)
		}
		if gitRepoMatch(curSrc.RepoURL, repoOwner, repoName) {
			newRevision = revision
		}
		revisions = append(revisions, newRevision)
		positions = append(positions, i+1)
	}
	return getApplicationChanges(ctx, appCur, "", revisions, positions)
}

// Called by processEvent() in main.go to fetch matching ArgoCD applications (based on repo owner & name)
// and return their manifests.
func GetApplicationChanges(ctx context.Context, eventInfo webhook.EventInfo) ([]ApplicationResourcesWithChanges, error) {
	log.Trace().Msgf("GetApplicationChanges(%+v)", eventInfo)
	var appResList []ApplicationResourcesWithChanges
	argoApps, err := listApplications(ctx)
	if err != nil {
		return appResList, err
	}
	log.Trace().Msgf("listApplications() returned %d items", len(argoApps.Items))
	if len(argoApps.Items) == 0 {
		return appResList, fmt.Errorf("empty ArgoCD app list")
	}
	appLookup := appListToMap(argoApps.Items)
	apps, err := filterApplications(argoApps.Items, eventInfo, false)
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

	multiSrcAppNamesDiffed := []string{}
	for _, app := range apps {
		log.Info().Msgf("Generating application diff for ArgoCD App '%s' w/ revision %s", app.ObjectMeta.Name, eventInfo.Sha)
		//app, err = getApplication(ctx, app.ObjectMeta.Name)
		//if err != nil {
		//	appResChanges.WarnStr = fmt.Sprintf("Failed to refresh application %s: %s", app.ObjectMeta.Name, err.Error())
		//	continue
		//}
		appResChanges, err := getApplicationChanges(ctx, &app, eventInfo.Sha, nil, nil)
		if err != nil {
			appResChanges.WarnStr = fmt.Sprintf("Failed to diff application %s: %s", app.ObjectMeta.Name, err.Error())
			appResList = append(appResList, appResChanges)
		} else if len(appResChanges.ChangedResources) > 0 {
			appResList = append(appResList, appResChanges)
			appsWithChanges, err := argoAppsWithChanges(ctx, app.ObjectMeta.Name, appResChanges.ChangedResources, eventInfo.Sha)
			if err != nil {
				log.Warn().Err(err).Msgf("Unable to determine if argo app %s has other argo apps with changes", app.ObjectMeta.Name)
			} else {
				// diff matching multi-source application
				log.Info().Msgf("%d Argo applications detected to have changes via %s", len(appsWithChanges), app.ObjectMeta.Name)
				for _, subApp := range appsWithChanges {
					if subAppCur, ok := appLookup[subApp.ObjectMeta.Name]; ok {
						multiSrcAppNamesDiffed = append(multiSrcAppNamesDiffed, subApp.ObjectMeta.Name)
						subAppResChanges, err := getMultiSrcAppChanges(ctx, &subAppCur, &subApp, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha)
						if err != nil {
							subAppResChanges.WarnStr = fmt.Sprintf("Failed to diff application %s: %s", subApp.ObjectMeta.Name, err.Error())
						} else if len(subAppResChanges.ChangedResources) > 0 {
							appResList = append(appResList, subAppResChanges)
						}
					} else {
						log.Info().Msgf("Application %s not found in current ArgoCD app list", subApp.ObjectMeta.Name)
					}
				}
			}
		}
	}
	// re-filter applications, except this time with multi-source
	apps, err = filterApplications(argoApps.Items, eventInfo, true)
	if err != nil {
		return appResList, err
	}
	log.Debug().Msgf("Matching multi-source apps: %s", func() (s string) {
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
		if slices.Contains(multiSrcAppNamesDiffed, app.ObjectMeta.Name) {
			log.Debug().Msgf("Skipping multi-source %s, we already diff'ed it", app.ObjectMeta.Name)
			continue
		}
		log.Info().Msgf("Generating application diff for multi-source ArgoCD App '%s' w/ revision %s", app.ObjectMeta.Name, eventInfo.Sha)
		revList := []string{}
		srcPos := []int{}
		for i, appSrc := range app.Spec.GetSources() {
			if gitRepoMatch(appSrc.RepoURL, eventInfo.RepoOwner, eventInfo.RepoName) {
				revList = append(revList, eventInfo.Sha)
				srcPos = append(srcPos, i+1)
			}
		}
		appResChanges, err := getApplicationChanges(ctx, &app, "", revList, srcPos)
		if err != nil {
			appResChanges.WarnStr = fmt.Sprintf("Failed to diff application %s: %s", app.ObjectMeta.Name, err.Error())
			appResList = append(appResList, appResChanges)
		} else if len(appResChanges.ChangedResources) > 0 {
			appResList = append(appResList, appResChanges)
		}
	}

	return appResList, nil
}

// Returns a list of Applications whose git URLs match repo owner & name
// eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.RepoDefaultRef, eventInfo.ChangeRef, eventInfo.BaseRef string
func filterApplications(a []Application, eventInfo webhook.EventInfo, multiSource bool) ([]Application, error) {
	log.Trace().Msgf("filterApplications([%d apps], %+v)", len(a), eventInfo)
	var appList []Application
	for _, app := range a {
		var sources []ApplicationSource
		singleSrc := app.Spec.GetSource()
		pluralSrc := app.Spec.GetSources()
		// GetSources() helper always returns a source, so check length of Sources slice
		if multiSource && len(app.Spec.Sources) > 0 {
			sources = pluralSrc
		}
		// GetSource() helper always returns a source, so check Source pointer
		if !multiSource && app.Spec.Source != nil {
			sources = []ApplicationSource{singleSrc}
		}
		for _, appSpecSource := range sources {
			if checkSource(appSpecSource, app.ObjectMeta.Name, eventInfo, app.Spec.SyncPolicy != nil && app.Spec.SyncPolicy.Automated != nil) {
				appList = append(appList, app)
				continue
			}
		}
	}
	if len(eventInfo.ChangedFiles) > 0 {
		log.Debug().Msg("Attempting to filter applications based on manifest-generate-paths annotation")
		return FilterApplicationsByPath(appList, eventInfo.ChangedFiles), nil
	}
	log.Debug().Msg("No changed files in event info; skipping check for manifest-generate-paths")
	return appList, nil
}

func gitRepoMatch(repoUrl, repoOwner, repoName string) bool {
    const githubHost = "github.com"
    candidates := []string{
        fmt.Sprintf("%s/%s/%s.git", githubHost, repoOwner, repoName),
        fmt.Sprintf("%s:%s/%s.git", githubHost, repoOwner, repoName),
        fmt.Sprintf("%s/%s/%s", githubHost, repoOwner, repoName),
        fmt.Sprintf("%s:%s/%s", githubHost, repoOwner, repoName),
    }
    log.Debug().Msgf("gitRepoMatch() - matching candidates: %v", candidates)
    for _, candidate := range candidates {
        if strings.HasSuffix(repoUrl, candidate) {
            return true
        }
    }
    return false
}

func checkSource(appSpecSource ApplicationSource, appName string, eventInfo webhook.EventInfo, automatedSync bool) bool {
	baseRef := eventInfo.BaseRef
	changeRef := eventInfo.ChangeRef
	repoDefaultRef := eventInfo.RepoDefaultRef
	log.Trace().Msgf("checkSource() - appname: %s (autosync %t)", appName, automatedSync)
	log.Trace().Msgf("checkSource() - appSpecSource: %+v", appSpecSource)
	log.Trace().Msgf("checkSource() - eventInfo: %+v", eventInfo)
	if !gitRepoMatch(appSpecSource.RepoURL, eventInfo.RepoOwner, eventInfo.RepoName) {
		log.Debug().Msgf("Filtering application %s: RepoURL %s doesn't mach owner/repo %s/%s", appName, appSpecSource.RepoURL, eventInfo.RepoOwner, eventInfo.RepoName)
		return false
	}
	if baseRef != "" {
		// Processing a PR ...
		if appSpecSource.TargetRevision == "HEAD" && baseRef != repoDefaultRef {
			// filter application if argo targets repo default (eg: main) and PR is not targetting main
			log.Debug().Msgf("Filtering application %s: Target Rev is HEAD; baseRef %s != repoDefaultRef %s", appName, baseRef, repoDefaultRef)
			return false
		}
		if appSpecSource.TargetRevision != "HEAD" && baseRef != appSpecSource.TargetRevision {
			// filter application if argo doesn't target repo default (eg: main)  and PR is not targetting that branch
			log.Debug().Msgf("Filtering application %s: baseRef %s != Target Rev %s", appName, baseRef, appSpecSource.TargetRevision)
			return false
		}
	} else {
		// processing a push
		// eg: refs/heads/main -> main
		changeRef = strings.TrimPrefix(changeRef, "refs/heads/")
		// filter out apps where auto-sync is enabled for the branch of the push
		if appSpecSource.TargetRevision == "HEAD" && changeRef == repoDefaultRef && automatedSync {
			log.Debug().Msgf("Filtering auto-sync application %s: Target Rev is HEAD; changeRef %s == repoDefaultRef %s", appName, changeRef, repoDefaultRef)
			return false
		}
		if appSpecSource.TargetRevision != "HEAD" && changeRef == appSpecSource.TargetRevision && automatedSync {
			log.Debug().Msgf("checkSource() - Filtering auto-sync application %s: changeRef %s = Target Rev %s", appName, changeRef, appSpecSource.TargetRevision)
			return false
		}
	}
	log.Debug().Msgf("checkSource() MATCH! application %s: changeRef %s = Target Rev %s", appName, changeRef, appSpecSource.TargetRevision)
	return true
}

func manifestIsArgoApplication(manifest K8sManifest) bool {
	name := manifest.Unstruct.GetName()
	kind := manifest.Unstruct.GetKind()
	apiVersion := manifest.Unstruct.GetAPIVersion()
	log.Trace().Msgf("manifestIsArgoApplication() called - %s/%s %s: %+v", apiVersion, kind, name, manifest.Unstruct)
	if kind == argoApplicationApiKind {
		log.Trace().Msgf("manifestIsArgoApplication() - found a %s", argoApplicationApiKind)
		apiVersionSplit := strings.Split(apiVersion, "/")
		if apiVersionSplit[0] == argoApplicationApiGroup {
			log.Debug().Msgf("manifestIsArgoApplication() - found a %s/%s in manifest", argoApplicationApiGroup, argoApplicationApiKind)
			return true
		}
	}
	return false
}

func genericManifestToArgoApplication(manifest K8sManifest) (Application, error) {
	log.Trace().Msgf("genericManifestToArgoApplication() converting generic manifest: %+v", manifest.Unstruct)
	log.Trace().Msgf("genericManifestToArgoApplication() converting yaml: %s", manifest.YamlSrc)
	var app Application
	err := yaml.Unmarshal(manifest.YamlSrc, &app)
	if err != nil {
		return app, err
	}
	log.Trace().Msgf("genericManifestToArgoApplication() returning Application: %+v", app)
	return app, nil
}

func argoAppsWithChanges(ctx context.Context, appName string, appResources []AppResource, revision string) ([]Application, error) {
	log.Trace().Msgf("argoAppsWithChanges() scanning %s at %s for argo apps", appName, revision)
	argoAppNamesFound := []string{}
	argoApps := []Application{}
	// look through app resource changes for argoproj.io Applications
	for _, appRes := range appResources {
		log.Trace().Msgf("argoAppsWithChanges(%s) - checking changed resource +++ %s/%s %s +++", appName, appRes.Group, appRes.Kind, appRes.Name)
		if appRes.Group == argoApplicationApiGroup && appRes.Kind == argoApplicationApiKind {
			log.Debug().Msgf("argoAppsWithChanges(%s) %s is an argo app (%s/%s)", appName, appRes.Name, argoApplicationApiGroup, argoApplicationApiKind)
			argoAppNamesFound = append(argoAppNamesFound, appRes.Name)
		}
	}
	if len(argoAppNamesFound) == 0 {
		// bail out if no applications have been found
		log.Debug().Msgf("argoAppsWithChanges() not argo apps found in %s", appName)
		return argoApps, nil
	}
	// generate full manifests for our application at the specified revision
	log.Debug().Msgf("argoAppsWithChanges(%s) - getting manifests at revision %s", appName, revision)
	manifests, err := getApplicationManifests(ctx, appName, revision)
	if err != nil {
		log.Debug().Err(err).Msgf("argoAppsWithChanges() - getApplicationManifests(%s, %s) failed", appName, revision)
		return argoApps, err
	}
	// look through resulting manifests for the argo apps found above
	for i, manifest := range manifests {
		if !manifestIsArgoApplication(manifest) {
			log.Trace().Msgf("argoAppsWithChanges(%s) - manifest at index %d is not an argo app", appName, i)
			continue
		}
		log.Debug().Msgf("argoAppsWithChanges(%s) - converting manifest at index %d to an Application", appName, i)
		app, err := genericManifestToArgoApplication(manifest)
		if err != nil {
			log.Error().Err(err).Msg("Detected an argo application, but Unable to convert")
		} else {
			name := app.ObjectMeta.Name
			numSrcs := len(app.Spec.GetSources())
			log.Trace().Msgf("argoAppsWithChanges(%s): argoApp %s w/ %d sources", appName, name, numSrcs)
			if slices.Contains(argoAppNamesFound, name) && numSrcs > 0 {
				// only return multi-source apps that have changes
				argoApps = append(argoApps, app)
			}
		}
	}
	return argoApps, nil
}
