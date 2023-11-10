package argocd

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

func GetApplicationResources(repoOwner, repoName, repoDefaultRef, revision, changeRef string) ([]ApplicationResources, error) {
	var appResList []ApplicationResources
	payload, err := fetchApplications()
	if err != nil { return appResList, err }
	apps, err := decodeApplicationListPayload(payload)
	if err != nil { return appResList, err }
	matchingApps, err := filterApplications(apps, repoOwner, repoName, repoDefaultRef, changeRef)
	if err != nil { return appResList, err }
	for _, app := range apps {
		appName := app.Metadata.Name
		payload, err = fetchAppRefresh(appName)
		if err != nil { return appResList, err }
		app, err = decodeApplicationRefreshPayload(payload)
		if err != nil { return appResList, err }
		payload, err = fetchManagedResources(appName, revision)
		if err != nil { return appResList, err }
		appRes, err := decodeManagedResources(payload)
		if err != nil { return appResList, err }
		appResList = append(appResList, ApplicationResources{ArgoApp: app, Resources: appRes})
	}
	if err != nil { return appResList, err }
	log.Debug().Msgf("Matching apps: %v", matchingApps)
	return appResList, nil
}

func filterApplications(a []Application, repoOwner, repoName, repoDefaultRef, changeRef string) ([]Application, error) {
	var appList []Application
	ghMatch1 := fmt.Sprintf("github.com/%s/%s.git", repoOwner, repoName)
	ghMatch2 := fmt.Sprintf("github.com/%s/%s", repoOwner, repoName)
	for _, app := range a {
		if (!strings.HasSuffix(app.Spec.Source.RepoURL, ghMatch1) && !strings.HasSuffix(app.Spec.Source.RepoURL, ghMatch2)) {
			log.Debug().Msgf("Filtering application %s: RepoURL %s doesn't much %s or %s", app.Metadata.Name, app.Spec.Source.RepoURL, ghMatch1, ghMatch2)
			continue
		}
		if (app.Spec.Source.TargetRevision == "HEAD" && changeRef == repoDefaultRef) {
			log.Debug().Msgf("Filtering application %s: Target Rev is HEAD; changeRef %s == repoDefaultRef %s", app.Metadata.Name, changeRef, repoDefaultRef)
			continue
		}
		if changeRef == app.Spec.Source.TargetRevision {
			log.Debug().Msgf("Filtering application %s: changeRef %s = Target Rev %s", app.Metadata.Name, changeRef, app.Spec.Source.TargetRevision)
			continue
		}
		appList = append(appList, app)
	}
	return appList, nil
}
