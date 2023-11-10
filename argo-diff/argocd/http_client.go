package argocd

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
)

var (
	httpClient http.Client
	baseURL string
	bearerToken string
)

func init() {
	httpClient = http.Client{}
	baseURL = os.Getenv("ARGOCD_BASE_URL")
	bearerToken = os.Getenv("ARGOCD_AUTH_TOKEN")
}

func httpGet(endpoint string) ([]byte, error) {
	if (baseURL == "" || bearerToken == "") {
		log.Info().Msg("ArgoCD base url and/or auth token are not set; can't perform httpGet()")
		return nil, fmt.Errorf("ARGOCD_BASE_URL and/or ARGOCD_AUTH_TOKEN not configured")
	}
	url := baseURL+endpoint
	log.Info().Msg("Performing HTTP GET of "+url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+bearerToken)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	return body, err
}

func fetchApplications() ([]byte, error) {
	log.Debug().Msg("fetchApplications() called")
	return httpGet("/api/v1/applications")
}

func fetchAppRefresh(appName string) ([]byte, error) {
	log.Debug().Msgf("fetchAppRefresh(%s) called", appName)
	return httpGet(fmt.Sprintf("/api/v1/applications/%s?refresh=normal", appName))
}

func fetchManagedResources(appName, revision string)  ([]byte, error) {
	log.Debug().Msgf("fetchManagedResources(%s, %s) called", appName, revision)
	endpoint := fmt.Sprintf("/api/v1/applications/%s/managed-resources?fields=items.normalizedLiveState%%2Citems.predictedLiveState%%2Citems.group%%2Citems.kind%%2Citems.namespace%%2Citems.name&version=%s",appName, revision)
	return httpGet(endpoint)
}
