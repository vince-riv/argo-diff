package argocd

import (
	"encoding/json"

	"github.com/rs/zerolog/log"
)

type Application struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Source struct {
			RepoURL        string `json:"repoURL"`
			Path           string `json:"path"`
			TargetRevision string `json:"targetRevision"`
		} `json:"source"`
		Destination struct {
			Server    string `json:"server"`
			Namespace string `json:"namespace"`
		} `json:"destination"`
		Project string `json:"project"`
	} `json:"spec"`
	Status struct {
		Sync struct {
			Status   string `json:"status"`
			Revision string `json:"revision,omitempty"`
		} `json:"sync"`
		Health struct {
			Status string `json:"status"`
		} `json:"health"`
	} `json:"status"`
}

type ApplicationList struct {
	Metadata struct {
		ResourceVersion string `json:"resourceVersion"`
	} `json:"metadata"`
	Items []Application `json:"items"`
}

type Manifests struct {
	Manifests  []string `json:"manifests"`
	Revision   string   `json:"revision"`
	SourceType string   `json:"sourceType"`
}

//type ManagedResource struct {
//	Kind string `json:"kind"`
//	Name string `json:"name"`
//	Namespace string `json:"namespace"`
//	NormalizedLiveState string `json:"normalizedLiveState"`
//	PredictedLiveState string `json:"predictedLiveState"`
//}
//
//type ManagedResources struct {
//	Items []ManagedResource `json:"items"`
//}

type ApplicationManifests struct {
	ArgoApp          *Application
	CurrentManifests *Manifests
	NewManifests     *Manifests
	Error            *ErrorPayload
}

type ErrorPayload struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func decodeApplicationListPayload(payload []byte) ([]Application, error) {
	var appList ApplicationList
	if err := json.Unmarshal(payload, &appList); err != nil {
		log.Error().Err(err).Msg("Error decoding ApplicationList payload")
		return appList.Items, err
	}
	return appList.Items, nil
}

func decodeApplicationRefreshPayload(payload []byte) (Application, error) {
	var app Application
	if err := json.Unmarshal(payload, &app); err != nil {
		log.Error().Err(err).Msg("Error decoding Application refresh payload")
		return app, err
	}
	return app, nil
}

func decodeManifestsPayload(payload []byte) (Manifests, error) {
	var m Manifests
	if err := json.Unmarshal(payload, &m); err != nil {
		log.Error().Err(err).Msg("Error decoding Application Manifests payload")
		return m, err
	}
	return m, nil
}

//func decodeManagedResources(payload []byte) (ManagedResources, error) {
//	var ar ManagedResources
//	if err := json.Unmarshal(payload, &ar); err != nil {
//		log.Error().Err(err).Msg("Error decoding Managed Resources payload")
//		return ar, err
//	}
//	return ar, nil
//}

func decodeErrorPayload(payload []byte) ErrorPayload {
	errPayload := ErrorPayload{
		Error:   "Unknown",
		Code:    -1,
		Message: "Unknown error",
	}

	if err := json.Unmarshal(payload, &errPayload); err != nil {
		log.Error().Err(err).Msg("Error decoding ApplicationList payload")
	}
	return errPayload
}
