package argocd

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Custom Application struct with only the fields we need
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ApplicationSpec   `json:"spec"`
	Status            ApplicationStatus `json:"status,omitempty"`
}

type ApplicationSpec struct {
	Source     *ApplicationSource   `json:"source,omitempty"`
	Sources    []ApplicationSource  `json:"sources,omitempty"`
	SyncPolicy *SyncPolicy         `json:"syncPolicy,omitempty"`
}

type ApplicationSource struct {
	RepoURL        string `json:"repoURL"`
	TargetRevision string `json:"targetRevision"`
	Path           string `json:"path,omitempty"`
	Chart          string `json:"chart,omitempty"`
	Ref            string `json:"ref,omitempty"`
}

type SyncPolicy struct {
	Automated *SyncPolicyAutomated `json:"automated,omitempty"`
}

type SyncPolicyAutomated struct {
	Prune    bool `json:"prune,omitempty"`
	SelfHeal bool `json:"selfHeal,omitempty"`
}

type ApplicationStatus struct {
	Sync   SyncStatus   `json:"sync,omitempty"`
	Health HealthStatus `json:"health,omitempty"`
}

type SyncStatus struct {
	Status string `json:"status,omitempty"`
}

type HealthStatus struct {
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

// GetSource returns the application's single source or the first source if multiple sources are defined
func (spec *ApplicationSpec) GetSource() ApplicationSource {
	if spec.Source != nil {
		return *spec.Source
	}
	if len(spec.Sources) > 0 {
		return spec.Sources[0]
	}
	return ApplicationSource{}
}

// GetSources returns the application's sources. If single source is defined, it returns a slice with that source.
// If multiple sources are defined, it returns the sources slice.
func (spec *ApplicationSpec) GetSources() []ApplicationSource {
	if len(spec.Sources) > 0 {
		return spec.Sources
	}
	if spec.Source != nil {
		return []ApplicationSource{*spec.Source}
	}
	return []ApplicationSource{}
}