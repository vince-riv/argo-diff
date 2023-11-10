package argocd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const testDataDir = "argocd_testdata"

const payloadAppList = "payload-GET-applications-brief.json"
const payloadAppRefresh = "payload-GET-application-refresh.json"
const payloadManagedResources = "payload-GET-managed-resources.json"

func readFileToByteArray(fileName string) ([]byte, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("Error getting current working directory: %w", err)
	}

	filePath := filepath.Join(workingDir, testDataDir, fileName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, filePath, fmt.Errorf("error reading file '%s': %w", filePath, err)
	}

	return data, filePath, nil
}

func checkArgoDiffAppProperties(a Application, t *testing.T) {
	if a.Metadata.Name == "" { t.Error("Empty name") }
	if a.Metadata.Namespace == "" { t.Error("Empty Namespace") }
	if a.Spec.Source.RepoURL == "" { t.Error("Empty RepoURL") }
	if a.Spec.Source.Path == "" { t.Error("Empty source path") }
	if a.Spec.Source.TargetRevision == "" { t.Error("Empty source revision") }
	if a.Spec.Destination.Server == "" { t.Error("Empty destination") }
	if a.Spec.Destination.Namespace == "" { t.Error("Empty destination namespace") }
	if a.Spec.Project == "" { t.Error("Empty project") }
	if a.Status.Sync.Status == "" { t.Error("Empty Sync Status") }
	if a.Status.Sync.Revision == "" { t.Error("Empty Sync Revision") }
	if a.Status.Health.Status == "" { t.Error("Empty Health Status") }
}

func TestLoadApplicationList(t *testing.T) {
	payload, _, err := readFileToByteArray(payloadAppList)
	if err != nil {
		t.Errorf("Failed to read %s: %v", payloadAppList, err)
	}
	apps, err := decodeApplicationListPayload(payload)
	if err != nil {
		t.Errorf("decodeApplicationListPayload() failed: %s", err)
	}
	for _, app := range apps {
		checkArgoDiffAppProperties(app, t)
	}
}

func TestLoadApplicationRefresh(t *testing.T) {
	payload, _, err := readFileToByteArray(payloadAppRefresh)
	if err != nil {
		t.Errorf("Failed to read %s: %v", payloadAppRefresh, err)
	}
	a, err := decodeApplicationRefreshPayload(payload)
	if err != nil {
		t.Errorf("decodeApplicationRefreshPayload() failed: %s", err)
	}
	if a.Metadata.Name != "argo-diff" { t.Error("Name not argo-diff") }
	if a.Metadata.Namespace != "argocd" { t.Error("Namespace not argocd") }
	if a.Spec.Source.RepoURL != "ssh://git@github.com/vince-riv/argo-diff.git" { t.Error("Bad RepoURL") }
	if a.Spec.Source.Path != "k8s" { t.Error("Bad source path") }
	if a.Spec.Source.TargetRevision != "HEAD" { t.Error("Bad source revision") }
	if a.Spec.Destination.Server != "https://kubernetes.default.svc" { t.Error("Bad destination") }
	if a.Spec.Destination.Namespace != "argocd" { t.Error("Bad destination namespace") }
	if a.Spec.Project != "argocd-extras" { t.Error("Bad project") }
	if a.Status.Sync.Status != "OutOfSync" { t.Error("Bad Sync Status") }
	if a.Status.Sync.Revision != "0341d95b8c70dc5f555fc8fe337e14b5496ff092" { t.Error("Bad Sync Revision") }
	if a.Status.Health.Status != "Healthy" { t.Error("Bad Health Status") }
}

func TestLoadManagedResources(t *testing.T) {
	payload, _, err := readFileToByteArray(payloadManagedResources)
	if err != nil {
		t.Errorf("Failed to read %s: %v", payloadManagedResources, err)
	}
	mrList, err := decodeManagedResources(payload)
	if err != nil {
		t.Errorf("decodeManagedResources() failed: %s", err)
	}
	
	if len(mrList.Items) == 0 {
		t.Errorf("No items found")
	}

	emptyKinds := 0
	emptyNames := 0
	emptyNamespaces := 0
	emptyNormalizedLiveStates := 0
	emptyPredictedLiveStates := 0

	for _, mr := range mrList.Items {
		if mr.Kind == "" { emptyKinds++ }
		if mr.Name == "" { emptyNames++ }
		if mr.Namespace == "" { emptyNamespaces++ }
		if mr.NormalizedLiveState == "" { emptyNormalizedLiveStates++ }
		if mr.PredictedLiveState == "" { emptyPredictedLiveStates++ }
	}

	if emptyKinds > 0 { t.Errorf("%d resources with empty Kind", emptyKinds) }
	if emptyNames > 0 { t.Errorf("%d resources with empty Name", emptyNames) }
	if emptyNamespaces > 0 { t.Errorf("%d resources with empty Namespace", emptyNamespaces) }
	if emptyNormalizedLiveStates > 0 { t.Errorf("%d resources with empty NormalizedLiveState", emptyNormalizedLiveStates) }
	if emptyPredictedLiveStates > 0 { t.Errorf("%d resources with empty PredictedLiveState", emptyPredictedLiveStates) }
}


