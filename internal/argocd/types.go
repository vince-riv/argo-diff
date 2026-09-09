package argocd

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type AppResource struct {
	ApiVersion string
	Group      string
	Kind       string
	Namespace  string
	Name       string
	DiffStr    string
}

type ApplicationResourcesWithChanges struct {
	ArgoApp          *Application
	ChangedResources []AppResource
	// WarnStr is fatal: the application's diff itself failed, so there are no
	// trustworthy ChangedResources to show. The reporter suppresses this app's
	// diffs, counts it as an error, and fails the run.
	WarnStr string
	// NoticeStr is advisory: the diff succeeded and ChangedResources are good,
	// but something alongside it degraded (eg: this app-of-apps' children
	// couldn't be enumerated). The reporter renders it above this app's diffs
	// and the run still succeeds.
	NoticeStr string
}

type K8sManifest struct {
	Unstruct unstructured.Unstructured
	YamlSrc  []byte
}
