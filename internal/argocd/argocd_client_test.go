package argocd

import (
	"testing"
)

var lokiClusterRoleDiff = `===== rbac.authorization.k8s.io/ClusterRoleBinding /loki-clusterrolebinding ======
--- /var/folders/4h/55t7qz1s27d2dpzp1dvcm5h40000gp/T/argocd-diff1365729858/loki-clusterrolebinding-live.yaml	2024-12-15 21:55:28
+++ /var/folders/4h/55t7qz1s27d2dpzp1dvcm5h40000gp/T/argocd-diff1365729858/loki-clusterrolebinding	2024-12-15 21:55:28
@@ -5,9 +5,9 @@
     app.kubernetes.io/instance: loki
     app.kubernetes.io/managed-by: Helm
     app.kubernetes.io/name: loki
-    app.kubernetes.io/version: 3.2.0
+    app.kubernetes.io/version: 3.3.1
     argocd.argoproj.io/instance: loki
-    helm.sh/chart: loki-6.18.0
+    helm.sh/chart: loki-6.23.0
   managedFields:
   - apiVersion: rbac.authorization.k8s.io/v1
     fieldsType: FieldsV1
`

var argocdCliVersionOutput = `argocd: v2.13.1+dc12345
  BuildDate: 2024-12-11T19:59:16Z
  GitCommit: dc43124058130db9a747d141d86d7c2f4aac7bf9
  GitTreeState: clean
  GoVersion: go1.23.4
  Compiler: gc
  Platform: darwin/arm64
argocd-server: v2.13.2+dc43124
  BuildDate: 2024-12-11T18:37:15Z
  GitCommit: dc43124058130db9a747d141d86d7c2f4aac7bf9
  GitTreeState: clean
  GoVersion: go1.23.1
  Compiler: gc
  Platform: linux/amd64
  Kustomize Version: v5.4.3 2024-07-19T16:40:33Z
  Helm Version: v3.15.4+gfa9efb0
  Kubectl Version: v0.31.0
  Jsonnet Version: v0.20.0
`

func TestParseArgoCDVersion(t *testing.T) {
	_, _, err := parseArgoCDVersion([]byte("garbage"))
	if err == nil {
		t.Error("Expected an error from garbage test string")
	}
	clientV, serverV, err := parseArgoCDVersion([]byte(argocdCliVersionOutput))
	if err != nil {
		t.Error("Unexpected an error from valid test string")
	}
	if clientV != "v2.13.1" {
		t.Errorf("Client version %s is not expected (%s)", clientV, "v2.13.1")
	}
	if serverV != "v2.13.2" {
		t.Errorf("Server version %s is not expected (%s)", clientV, "v2.13.2")
	}
}

func TestExtractFirstLin(t *testing.T) {
	firstLine, remaining := extractFirstLine(lokiClusterRoleDiff)
	expectedFirstLine := "===== rbac.authorization.k8s.io/ClusterRoleBinding /loki-clusterrolebinding ======"
	if firstLine != expectedFirstLine {
		t.Errorf("Expected first line '%s', got '%s'", expectedFirstLine, firstLine)
	}
	if len(remaining) == 0 {
		t.Error("Remaining string is empty")
	}
}

func TestExtractKubernetesFields(t *testing.T) {
	var group, kind, namespace, name string

	group, kind, namespace, name = extractKubernetesFields("===== policy/PodDisruptionBudget loki/loki-read ======")
	if group != "policy" {
		t.Errorf("Expected group 'policy', got %s", group)
	}
	if kind != "PodDisruptionBudget" {
		t.Errorf("Expected kind 'PodDisruptionBudget', got %s", kind)
	}
	if namespace != "loki" {
		t.Errorf("Expected namespace 'loki', got %s", namespace)
	}
	if name != "loki-read" {
		t.Errorf("Expected name 'loki-read', got %s", name)
	}

	group, kind, namespace, name = extractKubernetesFields("===== apps/StatefulSet loki/loki-chunks-cache ======")
	if group != "apps" {
		t.Errorf("Expected group 'apps', got %s", group)
	}
	if kind != "StatefulSet" {
		t.Errorf("Expected kind 'StatefulSet', got %s", kind)
	}
	if namespace != "loki" {
		t.Errorf("Expected namespace 'loki', got %s", namespace)
	}
	if name != "loki-chunks-cache" {
		t.Errorf("Expected name 'loki-chunks-cache', got %s", name)
	}

	group, kind, namespace, name = extractKubernetesFields("===== /ServiceAccount loki/loki-canary ======")
	if group != "" {
		t.Errorf("Expected group '', got %s", group)
	}
	if kind != "ServiceAccount" {
		t.Errorf("Expected kind 'ServiceAccount', got %s", kind)
	}
	if namespace != "loki" {
		t.Errorf("Expected namespace 'loki', got %s", namespace)
	}
	if name != "loki-canary" {
		t.Errorf("Expected name 'loki-canary', got %s", name)
	}

	group, kind, namespace, name = extractKubernetesFields("===== /Service loki/loki-results-cache ======")
	if group != "" {
		t.Errorf("Expected group '', got %s", group)
	}
	if kind != "Service" {
		t.Errorf("Expected kind 'Service', got %s", kind)
	}
	if namespace != "loki" {
		t.Errorf("Expected namespace 'loki', got %s", namespace)
	}
	if name != "loki-results-cache" {
		t.Errorf("Expected name 'loki-results-cache', got %s", name)
	}

	group, kind, namespace, name = extractKubernetesFields("===== rbac.authorization.k8s.io/ClusterRoleBinding /loki-clusterrolebinding ======")
	if group != "rbac.authorization.k8s.io" {
		t.Errorf("Expected group 'rbac.authorization.k8s.io', got %s", group)
	}
	if kind != "ClusterRoleBinding" {
		t.Errorf("Expected kind 'ClusterRoleBinding', got %s", kind)
	}
	if namespace != "" {
		t.Errorf("Expected namespace '', got %s", namespace)
	}
	if name != "loki-clusterrolebinding" {
		t.Errorf("Expected name 'loki-clusterrolebinding', got %s", name)
	}

	group, kind, namespace, name = extractKubernetesFields("===== rbac.authorization.k8s.io/ClusterRole /loki-clusterrole ======")
	if group != "rbac.authorization.k8s.io" {
		t.Errorf("Expected group 'rbac.authorization.k8s.io', got %s", group)
	}
	if kind != "ClusterRole" {
		t.Errorf("Expected kind 'ClusterRole', got %s", kind)
	}
	if namespace != "" {
		t.Errorf("Expected namespace '', got %s", namespace)
	}
	if name != "loki-clusterrole" {
		t.Errorf("Expected name 'loki-clusterrole', got %s", name)
	}
}

func TestAppManifestHelper(t *testing.T) {
	output, filepath, err := readFileToByteArray(appManifestsOutput1)
	if err != nil {
		t.Errorf("Failed to read %s: %v", filepath, err)
	}
	manifests, err := appManifestHelper(output)
	if err != nil {
		t.Errorf("appManifestHelper() failed on contents of %s: %v", filepath, err)
	}
	if len(manifests) != 1 {
		t.Errorf("appManifestHelper() - expected 1 manifest, got %d", len(manifests))
	}

	output, filepath, err = readFileToByteArray(appManifestsOutputArgoApps)
	if err != nil {
		t.Errorf("Failed to read %s: %v", filepath, err)
	}
	manifests, err = appManifestHelper(output)
	if err != nil {
		t.Errorf("appManifestHelper() failed on contents of %s: %v", filepath, err)
	}
	if len(manifests) != 4 {
		t.Errorf("appManifestHelper() - expected 4 manifests, got %d", len(manifests))
	}
}
