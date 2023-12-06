package argocd

import (
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

type AppResource struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
	YamlCur   string
	YamlNew   string
}

type ApplicationResourcesWithChanges struct {
	ArgoApp          *v1alpha1.Application
	TotalObjectCount int
	ChangedResources []AppResource
	WarnStr          string
}
