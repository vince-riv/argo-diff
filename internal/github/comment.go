package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v92/github"
	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/comment"
	"github.com/vince-riv/argo-diff/internal/config"
	"github.com/vince-riv/argo-diff/internal/scm"
)

var (
	commentClient      *github.Client
	appsClient         *github.Client
	commentClientIsApp bool
	commentLogin       string
	isGithubAction     bool
	mux                *sync.RWMutex
)

func init() {
	commentClientIsApp = false
	mux = &sync.RWMutex{}
	isGithubAction = os.Getenv("ARGO_DIFF_CI") != "true" && os.Getenv("GITHUB_ACTIONS") == "true"
	if isGithubAction {
		log.Debug().Msg("Running in github actions")
		// concurrent PRs running the action must not match each other's comments
		comment.SetIdentifierRef(os.Getenv("GITHUB_REF"))
	}
	// Create Github API client
	if githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"); githubPAT != "" {
		var err error
		commentClient, err = github.NewClient(github.WithAuthToken(githubPAT))
		if err != nil {
			log.Error().Err(err).Msg("Failed to create github client")
			return
		}
	} else if githubToken := os.Getenv("GITHUB_TOKEN"); githubToken != "" {
		var err error
		commentClient, err = github.NewClient(github.WithAuthToken(githubToken))
		if err != nil {
			log.Error().Err(err).Msg("Failed to create github client")
			return
		}
	} else {
		tr := http.DefaultTransport
		appId, err := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
		if err != nil {
			log.Error().Err(err).Msgf("Unable to parse %s", os.Getenv("GITHUB_APP_ID"))
			return
		}
		installId, err := strconv.ParseInt(os.Getenv("GITHUB_APP_INSTALLATION_ID"), 10, 64)
		if err != nil {
			log.Error().Err(err).Msgf("Unable to parse %s", os.Getenv("GITHUB_APP_INSTALLATION_ID"))
			return
		}
		privKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")
		atr, err := ghinstallation.NewAppsTransport(tr, appId, []byte(privKey))
		if err != nil {
			log.Error().Err(err).Msgf("Failed to create jwt transport: appId %d, privKey %s...", appId, privKey[:15])
			return
		}
		itr := ghinstallation.NewFromAppsTransport(atr, installId)
		commentClient, err = github.NewClient(github.WithHTTPClient(&http.Client{Transport: itr}))
		if err != nil {
			log.Error().Err(err).Msg("Failed to create github comment client")
			return
		}
		appsClient, err = github.NewClient(github.WithHTTPClient(&http.Client{Transport: atr}))
		if err != nil {
			log.Error().Err(err).Msg("Failed to create github apps client")
			return
		}
		commentClientIsApp = true
	}
}

// bypassGithubCheck reports whether the Github connectivity check and
// comment-author matching are bypassed via ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS.
// Read at call time, not cached in init(), so tests can t.Setenv it.
func bypassGithubCheck() bool {
	return config.BypassConnectivityCheck(config.ComponentGithub)
}

func ConnectivityCheck() error {
	if commentClient == nil {
		return errors.New("github client is not initialized")
	}
	if isGithubAction {
		log.Info().Msg("Running in github actions - skipping connectivity test")
		return nil
	}
	if bypassGithubCheck() {
		log.Warn().Msgf("Skipping Github connectivity test per %s", config.BypassEnvVar)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	log.Info().Msg("Calling Github API for a connectivity test")
	return getCommentUser(ctx)
}

// getCommentLogin returns the cached commentLogin under a read lock. Every
// read of commentLogin outside getCommentUser must go through this accessor
// so it can never race with the writes in getCommentUser.
func getCommentLogin() string {
	mux.RLock()
	defer mux.RUnlock()
	return commentLogin
}

// Populates commentLogin singleton with the Github user associated with our github client
func getCommentUser(ctx context.Context) error {
	if commentClient == nil {
		log.Error().Msg("Cannot call github API - I don't have a client set")
		return fmt.Errorf("no github commenter client")
	}
	// Hold the write lock across the whole check-then-call-then-set sequence
	// so two concurrent callers can't both observe an empty commentLogin and
	// both hit the Github API. This only serializes the first call(s) after
	// process start, since every call after that returns immediately below.
	mux.Lock()
	defer mux.Unlock()
	if commentLogin != "" {
		return nil
	}
	log.Debug().Msg("Calling Github API to determine comment user")
	if commentClientIsApp {
		app, resp, err := appsClient.Apps.Get(ctx, "")
		if resp != nil {
			log.Info().Msgf("%s received when calling client.Apps.Get() via go-github", resp.Status)
		}
		if err != nil {
			log.Error().Err(err).Msg("Unable to determine get my github app")
			return err
		}
		log.Trace().Msgf("Github App: %+v", app)
		if app == nil {
			log.Error().Msg("Empty app returned - not sure how I got here")
			return fmt.Errorf("empty app info")
		}
		appLogin := app.GetSlug()
		if appLogin == "" {
			log.Warn().Msg("Github App slug is empty - falling back to app name")
			appLogin = app.GetName()
		}
		if appLogin == "" {
			log.Error().Msg("Github App has neither slug nor name")
			return fmt.Errorf("empty app info")
		}
		commentLogin = appLogin + "[bot]"
	} else {
		user, resp, err := commentClient.Users.Get(ctx, "")
		if resp != nil {
			log.Info().Msgf("%s received when calling client.Users.Get() via go-github", resp.Status)
		}
		if err != nil {
			log.Error().Err(err).Msg("Unable to determine get my github user")
			return err
		}
		if user == nil || user.Login == nil {
			log.Error().Msg("Empty user returned - not sure how I got here")
			return fmt.Errorf("empty user info")
		}
		commentLogin = *user.Login
	}
	log.Info().Msgf("Github Comment user name: %s", commentLogin)
	return nil
}

// GetPullRequest fetches the specified pull request. A branch the API did not
// return is left empty in the result; see scm.ChangeRequest.
func GetPullRequest(ctx context.Context, owner, repo string, prNum int) (scm.ChangeRequest, error) {
	pr, resp, err := commentClient.PullRequests.Get(ctx, owner, repo, prNum)
	if resp != nil {
		log.Info().Msgf("%s received when calling commentClient.PullRequests.Get() via go-github", resp.Status)
	}
	if err != nil {
		log.Error().Err(err).Msgf("Unable to fetch pull request %s/%s#%d", owner, repo, prNum)
		return scm.ChangeRequest{}, err
	}
	return changeRequestFromPull(pr, prNum), nil
}

// changeRequestFromPull converts a go-github pull request. The getters are
// nil-safe, so a missing head or base yields empty fields.
func changeRequestFromPull(pr *github.PullRequest, prNum int) scm.ChangeRequest {
	return scm.ChangeRequest{
		Number:  prNum,
		HeadSHA: pr.GetHead().GetSHA(),
		HeadRef: pr.GetHead().GetRef(),
		BaseRef: pr.GetBase().GetRef(),
	}
}

// Returns list of files in a pull request
func ListPullRequestFiles(ctx context.Context, owner, repo string, prNum int) ([]string, error) {
	var fileList []string
	cfs, resp, err := commentClient.PullRequests.ListFiles(ctx, owner, repo, prNum, nil)
	if resp != nil {
		log.Info().Msgf("%s received when calling commentClient.PullRequests.ListFiles() via go-github", resp.Status)
	}
	if err != nil {
		return nil, err
	}
	for _, cf := range cfs {
		if cf != nil && cf.Filename != nil {
			fileList = append(fileList, *cf.Filename)
		} else {
			log.Warn().Msgf("nil value found in call to list files in pull request %s/%s#%d", owner, repo, prNum)
		}
	}
	return fileList, nil
}

//func saveResponse(v any, filename string) {
//	jsonData, err := json.Marshal(v)
//	if err != nil {
//		log.Warn().Err(err).Msg("Failed to call json.Marshal() in saveResponse()")
//		return
//	}
//	file, err := os.Create(filename)
//	if err != nil {
//		log.Warn().Err(err).Msg("Failed to call os.Create('output.json') in saveResponse()")
//		return
//	}
//	defer file.Close()
//	_, err = file.Write(jsonData)
//	if err != nil {
//		log.Warn().Err(err).Msg("Failed to call file.Write(jsonData) in saveResponse()")
//	}
//}
