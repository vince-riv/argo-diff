package argocd

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

func errorPayloadHelper(payload []byte, message string, code int) ErrorPayload {
	if payload != nil {
		return decodeErrorPayload(payload)
	}
	return ErrorPayload{
		Error:   message,
		Code:    code,
		Message: message,
	}
}

func GetApplicationManifests(repoOwner, repoName, repoDefaultRef, revision, changeRef string) ([]ApplicationManifests, error) {
	var appManList []ApplicationManifests
	payload, err := fetchApplications()
	if err != nil {
		return appManList, err
	}
	apps, err := decodeApplicationListPayload(payload)
	if err != nil {
		return appManList, err
	}
	apps, err = filterApplications(apps, repoOwner, repoName, repoDefaultRef, changeRef)
	if err != nil {
		return appManList, err
	}
	log.Debug().Msgf("Matching apps: %v", apps)
	for _, app := range apps {
		appName := app.Metadata.Name
		// Application Refresh [TODO: perform hard refresh?]
		payload, err = fetchAppRefresh(appName)
		if err != nil {
			errPayload := errorPayloadHelper(payload, "App Refresh Failed - see logs for more details", 10000)
			appManList = append(appManList, ApplicationManifests{ArgoApp: &app, Error: &errPayload})
			continue
		}
		refreshApp, err := decodeApplicationRefreshPayload(payload)
		if err != nil {
			errPayload := errorPayloadHelper(payload, "App Refresh Failed to decode - see logs for more details", 10001)
			appManList = append(appManList, ApplicationManifests{ArgoApp: &app, Error: &errPayload})
			continue
		}
		app = refreshApp
		// Fetch Current App Manifests
		payload, err = fetchManifests(appName, "")
		if err != nil {
			errPayload := errorPayloadHelper(payload, "Failed to Fetch App Manifests - see logs for more details", 10002)
			appManList = append(appManList, ApplicationManifests{ArgoApp: &app, Error: &errPayload})
			continue
		}
		curManifests, err := decodeManifestsPayload(payload)
		if err != nil {
			errPayload := errorPayloadHelper(payload, "App Manifests Failed to decode - see logs for more details", 10003)
			appManList = append(appManList, ApplicationManifests{ArgoApp: &app, Error: &errPayload})
			continue
		}
		// Fetch Predicted App Manifests
		payload, err = fetchManifests(appName, revision)
		if err != nil {
			errPayload := errorPayloadHelper(payload, "Failed to Fetch New App Manifests - see logs for more details", 10004)
			appManList = append(appManList, ApplicationManifests{ArgoApp: &app, CurrentManifests: &curManifests, Error: &errPayload})
			continue
		}
		newManifests, err := decodeManifestsPayload(payload)
		if err != nil {
			errPayload := errorPayloadHelper(payload, "New App Manifests Failed to decode - see logs for more details", 10005)
			appManList = append(appManList, ApplicationManifests{ArgoApp: &app, CurrentManifests: &curManifests, Error: &errPayload})
			continue
		}
		appManList = append(appManList, ApplicationManifests{ArgoApp: &app, CurrentManifests: &curManifests, NewManifests: &newManifests})
	}
	return appManList, nil
}

func filterApplications(a []Application, repoOwner, repoName, repoDefaultRef, changeRef string) ([]Application, error) {
	var appList []Application
	ghMatch1 := fmt.Sprintf("github.com/%s/%s.git", repoOwner, repoName)
	ghMatch2 := fmt.Sprintf("github.com/%s/%s", repoOwner, repoName)
	for _, app := range a {
		if !strings.HasSuffix(app.Spec.Source.RepoURL, ghMatch1) && !strings.HasSuffix(app.Spec.Source.RepoURL, ghMatch2) {
			log.Debug().Msgf("Filtering application %s: RepoURL %s doesn't much %s or %s", app.Metadata.Name, app.Spec.Source.RepoURL, ghMatch1, ghMatch2)
			continue
		}
		if app.Spec.Source.TargetRevision == "HEAD" && changeRef == repoDefaultRef {
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
