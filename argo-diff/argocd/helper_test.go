package argocd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFilterApplications(t *testing.T) {
	var a []Application

	result, _ := filterApplications(a, "o", "r", "m", "m")
	if len(result) != 0 {
		t.Error("Empty param didn't lead to empty result")
	}
	payload, _, err := readFileToByteArray(payloadAppList)
	if err != nil {
		t.Errorf("Failed to read %s: %v", payloadAppList, err)
	}
	a, err = decodeApplicationListPayload(payload)
	if err != nil {
		t.Errorf("decodeApplicationListPayload() failed: %s", err)
	}

	result, _ = filterApplications(a, "o", "r", "m", "m")
	if len(result) != 0 {
		t.Error("Unmatchable params didn't lead to empty result")
	}

	result, _ = filterApplications(a, "vince-riv", "argo-diff", "main", "main")
	if len(result) != 0 {
		t.Error("Push to main shouldn't have matched")
	}

	result, _ = filterApplications(a, "vince-riv", "argo-diff", "main", "dev")
	if len(result) != 1 {
		t.Error("Push to dev should have matched")
	}

	a[0].Spec.Source.TargetRevision = "main"
	a[1].Spec.Source.TargetRevision = "main"
	result, _ = filterApplications(a, "vince-riv", "argo-diff", "main", "main")
	if len(result) != 0 {
		t.Error("Push to main shouldn't have matched (targetRev main)")
	}

	result, _ = filterApplications(a, "vince-riv", "argo-diff", "main", "dev")
	if len(result) != 1 {
		t.Error("Push to dev should have matched (targetRev main)")
	}
}

func newHttpTestServer(t *testing.T) *httptest.Server {
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCode := http.StatusNotFound
		payload := []byte(`404 Page Not Found`)
		var filePath string
		var err error
		//println("r.URL.Path: " + r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/applications":
			statusCode = http.StatusOK
			payload, filePath, err = readFileToByteArray(payloadAppList)
		case "/api/v1/applications/argo-diff":
			q := r.URL.Query()
			refresh := q.Get("refresh")
			if refresh != "normal" {
				t.Errorf("Unexpected refresh query param: '%s'", refresh)
			}
			statusCode = http.StatusOK
			payload, filePath, err = readFileToByteArray(payloadAppRefresh)
		case "/api/v1/applications/argo-diff/manifests":
			q := r.URL.Query()
			revision := q.Get("revision")
			statusCode = http.StatusOK
			switch revision {
			case "":
				payload, filePath, err = readFileToByteArray(payloadManifests)
			case "0e292869801c52fa38d655596545e95953ac8e3e":
				payload, filePath, err = readFileToByteArray(payloadManifestsChange)
			case "ffffffffffffffffffffffffffffffffffffffff":
				statusCode = http.StatusInternalServerError
				payload, filePath, err = readFileToByteArray(payloadError)
			default:
				t.Errorf("Unexpected revision in query param: %s", revision)
				statusCode = http.StatusBadRequest
			}
		default:
			t.Errorf("Mock server not configured to serve path %s", r.URL.Path)
		}
		if err != nil {
			t.Errorf("Failed to load %s: %s", filePath, err)
			statusCode = http.StatusInternalServerError
		}
		w.WriteHeader(statusCode)
		w.Write(payload)
	}))
	return newServer
}

func TestGetAppManifestsGood(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	httpBaseUrl = server.URL
	httpBearerToken = "test1234"

	appManifests, err := GetApplicationManifests("vince-riv", "argo-diff", "main", "0e292869801c52fa38d655596545e95953ac8e3e", "dev")
	if err != nil {
		t.Errorf("GetApplicationManifests() with error: %s", err)
	}
	if len(appManifests) != 1 {
		t.Errorf("%d applications returned by GetApplicationManifests()", len(appManifests))
	}
	if appManifests[0].ArgoApp.Metadata.Name != "argo-diff" {
		t.Error("Unxpected argo app returned")
	}
	if appManifests[0].Error != nil {
		t.Error("Not expecting an error")
	}
	if len(appManifests[0].CurrentManifests.Manifests) != 4 {
		t.Error("Expected 4 manifests")
	}
	if len(appManifests[0].NewManifests.Manifests) != 4 {
		t.Error("Expected 4 new manifests")
	}
}

func TestGetAppManifestsBad(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	httpBaseUrl = server.URL
	httpBearerToken = "test1234"

	appManifests, err := GetApplicationManifests("vince-riv", "argo-diff", "main", "ffffffffffffffffffffffffffffffffffffffff", "dev")
	if err != nil {
		t.Errorf("GetApplicationManifests() with error: %s", err)
	}
	if len(appManifests) != 1 {
		t.Errorf("%d applications returned by GetApplicationManifests()", len(appManifests))
	}
	if appManifests[0].ArgoApp.Metadata.Name != "argo-diff" {
		t.Error("Unxpected argo app returned")
	}
	if appManifests[0].Error == nil {
		t.Error("Expecting an error; didn't get one")
	}
	if len(appManifests[0].CurrentManifests.Manifests) != 4 {
		t.Error("Expected 4 manifests")
	}
	if appManifests[0].NewManifests != nil {
		t.Error("Expected empty New Manifests")
	}
}
