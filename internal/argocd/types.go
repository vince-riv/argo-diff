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
	WarnStr          string
}

type K8sManifest struct {
	Unstruct unstructured.Unstructured
	YamlSrc  []byte
}
