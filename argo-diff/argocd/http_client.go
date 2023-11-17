package argocd

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
)

var (
	httpClient      http.Client
	httpBaseUrl     string
	httpBearerToken string
)

func init() {
	httpClient = http.Client{}
	httpBaseUrl = os.Getenv("ARGOCD_BASE_URL")
	httpBearerToken = os.Getenv("ARGOCD_AUTH_TOKEN")
}

func httpGet(endpoint string) ([]byte, error) {
	if httpBaseUrl == "" || httpBearerToken == "" {
		log.Info().Msg("ArgoCD base url and/or auth token are not set; can't perform httpGet()")
		return nil, fmt.Errorf("ARGOCD_BASE_URL and/or ARGOCD_AUTH_TOKEN not configured")
	}
	url := httpBaseUrl + endpoint
	log.Debug().Msg("Performing HTTP GET of " + url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+httpBearerToken)
	req.Header.Add("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 500 {
			log.Warn().Msgf("Received HTTP 500 from %s", url)
			body, _ := io.ReadAll(resp.Body)
			return body, fmt.Errorf("HTTP 500 returned from %s", url)
		} else {
			log.Warn().Msgf("Received HTTP %d from %s", resp.StatusCode, url)
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	}

	body, err := io.ReadAll(resp.Body)
	log.Info().Msgf("Received HTTP 200 from %s; content-length %d", url, len(body))
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

func fetchManifests(appName, revision string) ([]byte, error) {
	log.Debug().Msgf("fetchManifests(%s, %s) called", appName, revision)
	endpoint := fmt.Sprintf("/api/v1/applications/%s/manifests", appName)
	if revision != "" {
		endpoint = fmt.Sprintf("/api/v1/applications/%s/manifests?revision=%s", appName, revision)
	}
	return httpGet(endpoint)
}

//func fetchManagedResources(appName, revision string) ([]byte, error) {
//	log.Debug().Msgf("fetchManagedResources(%s, %s) called", appName, revision)
//	endpoint := fmt.Sprintf("/api/v1/applications/%s/managed-resources?fields=items.normalizedLiveState%%2Citems.predictedLiveState%%2Citems.group%%2Citems.kind%%2Citems.namespace%%2Citems.name&version=%s", appName, revision)
//	return httpGet(endpoint)
//}
