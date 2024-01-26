package argocd

/*
 * Simple http client for calling argocd
 */

// TODO handle pagination
// TODO accept context.Context to gracefully handle deadlines
// TODO cache some payloads in-memory (maybe?)

import (
	"context"
	"os"
	"strings"
	"time"

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
	serverAddr := os.Getenv("ARGOCD_SERVER_ADDR")
	insecure := strings.ToLower(os.Getenv("ARGOCD_SERVER_INSECURE")) == "true"
	plaintext := strings.ToLower(os.Getenv("ARGOCD_SERVER_PLAINTEXT")) == "true"
	httpBearerToken = os.Getenv("ARGOCD_AUTH_TOKEN")
	if serverAddr != "" && httpBearerToken != "" {
		setArgoClients(serverAddr, insecure, plaintext, httpBearerToken)
	} else {
		log.Warn().Msg("Initialized without ArgoCD server config")
	}
}

func setArgoClients(serverAddr string, insecure, plaintext bool, token string) {
	log.Info().Msgf("Creating new ArgoCD API Client; ServerAddr %s, Insecure %t, PlainText %t", serverAddr, insecure, plaintext)
	argocdApiClient = apiclient.NewClientOrDie(&apiclient.ClientOptions{
		ServerAddr: serverAddr,
		Insecure:   insecure,
		PlainText:  plaintext,
		AuthToken:  token,
	})
	_, applicationIf = argocdApiClient.NewApplicationClientOrDie()
	_, settingsIf = argocdApiClient.NewSettingsClientOrDie()
	_, projIf = argocdApiClient.NewProjectClientOrDie()
	log.Debug().Msg("ArgoCD API clients created")
}

func ConnectivityCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	log.Info().Msg("Calling ArgoCD to list applications for a connectivity test")
	_, err := listApplications(ctx)
	return err
}

func listApplications(ctx context.Context) (*v1alpha1.ApplicationList, error) {
	log.Trace().Msg("listApplications() called")
	apps, err := applicationIf.List(ctx, &applicationpkg.ApplicationQuery{})
	if err != nil {
		log.Error().Err(err).Msg("Application List failed")
		return nil, err
	}
	log.Trace().Msg("applicationIf.List() completed")
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
