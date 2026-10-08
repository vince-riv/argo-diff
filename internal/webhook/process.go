package webhook

import (
	"errors"
)

// EventInfo describes one change request event, whichever provider it came
// from. It is also the file format of -f event files and of the /dev endpoint.
type EventInfo struct {
	// Provider names the scm provider the event belongs to (eg: "github").
	// Empty means scm.DefaultProvider, so event files from before
	// multi-provider support keep working.
	Provider       string   `json:"provider"`
	Ignore         bool     `json:"ignore"`
	RepoOwner      string   `json:"owner"`
	RepoName       string   `json:"repo"`
	RepoDefaultRef string   `json:"default_ref"`
	Sha            string   `json:"commit_sha"`
	PrNum          int      `json:"pr"`
	ChangeRef      string   `json:"change_ref"`
	BaseRef        string   `json:"base_ref"`
	Refresh        bool     `json:"refresh"`
	ChangedFiles   []string `json:"changed_files,omitempty"`
}

func NewEventInfo() EventInfo {
	return EventInfo{
		Ignore:         true,
		RepoOwner:      "",
		RepoName:       "",
		RepoDefaultRef: "",
		Sha:            "",
		PrNum:          -1,
		ChangeRef:      "",
		BaseRef:        "",
		Refresh:        false,
	}
}

// Validate checks that a parsed event carries what processing needs. Sha and
// ChangeRef are only required when Refresh is false, since a refresh re-reads
// them from the provider.
func (e EventInfo) Validate() error {
	if e.RepoOwner == "" {
		return errors.New("missing repo owner in event info object")
	}
	if e.RepoName == "" {
		return errors.New("missing repo name in event info object")
	}
	if e.RepoDefaultRef == "" {
		return errors.New("missing default ref in event info object")
	}
	if !e.Refresh && e.Sha == "" {
		return errors.New("missing SHA in event info object")
	}
	if !e.Refresh && e.ChangeRef == "" {
		return errors.New("missing change ref in event info object")
	}
	return nil
}
