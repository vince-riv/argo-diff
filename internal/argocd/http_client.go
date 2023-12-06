package argocd

/*
 * Simple http client for calling argocd
 */

// TODO handle pagination
// TODO accept context.Context to gracefully handle deadlines
// TODO cache some payloads in-memory (maybe?)

import (
	"context"
	"encoding/json"
	"net/url"
	"os"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	projectpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	settingspkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/settings"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	repoapiclient "github.com/argoproj/argo-cd/v2/reposerver/apiclient"
	"github.com/rs/zerolog/log"
)

var (
	argocdApiClient apiclient.Client
	applicationIf   applicationpkg.ApplicationServiceClient
	settingsIf      settingspkg.SettingsServiceClient
	projIf          projectpkg.ProjectServiceClient
	httpBearerToken string
)

func init() {
	baseUrl := os.Getenv("ARGOCD_BASE_URL")
	httpBearerToken = os.Getenv("ARGOCD_AUTH_TOKEN")
	if baseUrl != "" && httpBearerToken != "" {
		setArgoClients(baseUrl, httpBearerToken)
	}
}

func setArgoClients(baseUrl, token string) {
	u, err := url.Parse(baseUrl)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to parse ARGOCD_BASE_URL '%s'", baseUrl)
	}
	argoInsecure := u.Scheme != "https"
	argocdApiClient = apiclient.NewClientOrDie(&apiclient.ClientOptions{
		ServerAddr: u.Host,
		Insecure:   argoInsecure,
		PlainText:  argoInsecure,
		AuthToken:  token,
	})
	_, applicationIf = argocdApiClient.NewApplicationClientOrDie()
	_, settingsIf = argocdApiClient.NewSettingsClientOrDie()
	_, projIf = argocdApiClient.NewProjectClientOrDie()
}

func listApplications(ctx context.Context) (*v1alpha1.ApplicationList, error) {
	apps, err := applicationIf.List(ctx, &applicationpkg.ApplicationQuery{})
	if err != nil {
		log.Error().Err(err).Msg("Application List failed")
		return nil, err
	}
	content, err := json.Marshal(apps)
	if err != nil {
		log.Error().Err(err).Msg("json.Marshal failed")
		return apps, nil
	}
	err = os.WriteFile("app-list.json", content, 0644)
	if err != nil {
		log.Error().Err(err).Msg("os.WriteFile() failed")
		return apps, nil
	}
	return apps, nil
}

func getApplication(ctx context.Context, appName, appNs string) (*v1alpha1.Application, error) {
	refreshType := string(v1alpha1.RefreshTypeNormal) // switch to RefreshTypeHard ?
	app, err := applicationIf.Get(ctx, &applicationpkg.ApplicationQuery{
		Name:         &appName,
		Refresh:      &refreshType,
		AppNamespace: &appNs,
	})
	if err != nil {
		log.Error().Err(err).Msgf("Get Argo application %s (namespace %s) failed", appName, appNs)
		return nil, err
	}
	return app, nil
}

func getManagedResources(ctx context.Context, appName, appNs string) (*applicationpkg.ManagedResourcesResponse, error) {
	resources, err := applicationIf.ManagedResources(ctx, &applicationpkg.ResourcesQuery{
		ApplicationName: &appName,
		AppNamespace:    &appNs,
	})
	if err != nil {
		log.Error().Err(err).Msgf("Get Argo managed-resources for %s (namespace %s) failed", appName, appNs)
		return nil, err
	}
	return resources, nil
}

func getAppManifests(ctx context.Context, appName, appNs, revision string) (*repoapiclient.ManifestResponse, error) {
	m, err := applicationIf.GetManifests(ctx, &applicationpkg.ApplicationManifestQuery{
		Name:         &appName,
		AppNamespace: &appNs,
		Revision:     &revision,
	})
	if err != nil {
		log.Error().Err(err).Msgf("Get Argo manifests for %s (namespace %s) @ %s failed", appName, appNs, revision)
		return nil, err
	}
	return m, nil
}

func getProject(ctx context.Context, projName string) (*projectpkg.DetailedProjectsResponse, error) {
	detailedProject, err := projIf.GetDetailedProject(ctx, &projectpkg.ProjectQuery{Name: projName})
	if err != nil {
		log.Error().Err(err).Msgf("Get Project details for %s failed", projName)
		return nil, err
	}
	return detailedProject, nil
}

func getSettings(ctx context.Context) (*settingspkg.Settings, error) {
	s, err := settingsIf.Get(ctx, &settingspkg.SettingsQuery{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch ArgoCD Settings")
		return nil, err
	}
	return s, nil
}
