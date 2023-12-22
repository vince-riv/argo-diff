package argocd

import (
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

type AppResource struct {
	ApiVersion string
	Group      string
	Kind       string
	Namespace  string
	Name       string
	YamlCur    string
	YamlNew    string
}

type ApplicationResourcesWithChanges struct {
	ArgoApp          *v1alpha1.Application
	TotalObjectCount int
	ChangedResources []AppResource
	WarnStr          string
}

const (
	SyncSkip    = -1
	SyncFail    = 0
	SyncSuccess = 1
)

type ApplicationSyncResult struct {
	ArgoApp           *v1alpha1.Application
	ManifestsFetched  bool
	ManifestsFetchMsg string
	SyncSuccess       int
	SyncMsg           string
}
