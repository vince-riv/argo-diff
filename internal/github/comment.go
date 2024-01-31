package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v58/github"
	"github.com/rs/zerolog/log"
)

var (
	commentClient      *github.Client
	appsClient         *github.Client
	commentClientIsApp bool
	commentLogin       string
	mux                *sync.RWMutex
)

const commentIdentifier = "<!-- comment produced by argo-diff -->"

func init() {
	commentClientIsApp = false
	mux = &sync.RWMutex{}
	// Create Github API client
	if githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"); githubPAT != "" {
		commentClient = github.NewClient(nil).WithAuthToken(githubPAT)
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
		commentClient = github.NewClient(&http.Client{Transport: itr})
		appsClient = github.NewClient(&http.Client{Transport: atr}) // /app endpoints need separate client
		commentClientIsApp = true
	}
}

func ConnectivityCheck() error {
	if commentClient == nil {
		return errors.New("github client is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	log.Info().Msg("Calling Github API for a connectivity test")
	return getCommentUser(ctx)
}

// Populates commentLogin singleton with the Github user associated with our github client
func getCommentUser(ctx context.Context) error {
	if commentClient == nil {
		log.Error().Msg("Cannot call github API - I don't have a client set")
		return fmt.Errorf("no github commenter client")
	}
	log.Debug().Msg("Calling Github API to determine comment user")
	mux.RLock()
	if commentLogin != "" {
		mux.RUnlock()
		return nil
	}
	mux.RUnlock()
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
		if app == nil || app.Name == nil {
			log.Error().Msg("Empty user returned - not sure how I got here")
			return fmt.Errorf("empty app info")
		}
		mux.Lock()
		commentLogin = *app.Name + "[bot]"
		mux.Unlock()
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
		mux.Lock()
		commentLogin = *user.Login
		mux.Unlock()
	}
	log.Info().Msgf("Github Comment user name: %s", commentLogin)
	return nil
}

// Gets the specified pull request
func GetPullRequest(ctx context.Context, owner, repo string, prNum int) (*github.PullRequest, error) {
	pr, resp, err := commentClient.PullRequests.Get(ctx, owner, repo, prNum)
	if resp != nil {
		log.Info().Msgf("%s received when calling client.Users.Get() via go-github", resp.Status)
	}
	if err != nil {
		log.Error().Err(err).Msgf("Unable to fetch pull request %s/%s#%d", owner, repo, prNum)
		return nil, err
	}
	return pr, nil
}

// Returns true if sha is HEAD of the pull request
func isPrHead(ctx context.Context, sha, owner, repo string, prNum int) bool {
	pr, err := GetPullRequest(ctx, owner, repo, prNum)
	if err != nil {
		log.Warn().Msgf("GetPullRequest() err'd - assuming %s is HEAD of %s/%s#%d", sha, owner, repo, prNum)
		return true
	}
	if pr.Head == nil {
		log.Warn().Msgf("%s/%s#%d has no HEAD - assuming %s is not HEAD", owner, repo, prNum, sha)
		return false
	}
	if pr.Head.SHA == nil {
		log.Warn().Msgf("SHA for PullRequestBranch of %s/%s#%d is nil - assuming %s is not HEAD", owner, repo, prNum, sha)
		return false
	}
	return sha == *pr.Head.SHA
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

// Returns a list of pointers to pull request comments previously generated by argo-diff.
// Returns an empty list if there is no matching comment
func getExistingComments(ctx context.Context, owner, repo string, prNum int) ([]*github.IssueComment, error) {
	sortOpt := "created"
	sortDirection := "asc"
	issueListCommentsOpts := github.IssueListCommentsOptions{
		Sort:      &sortOpt,
		Direction: &sortDirection,
	}
	var res []*github.IssueComment
	//var existingComment *github.IssueComment
	if commentClient == nil {
		log.Error().Msg("Cannot call github API - I don't have a client set")
		return nil, fmt.Errorf("no github commenter client")
	}
	getCommentUser(ctx)
	for i, checkComments := 0, true; checkComments; i++ {
		checkComments = false
		comments, resp, err := commentClient.Issues.ListComments(ctx, owner, repo, prNum, &issueListCommentsOpts)
		if resp != nil {
			log.Info().Msgf("%s received when calling commentClient.PullRequest.ListComments(%s, %s, %d, %v) via go-github", resp.Status, owner, repo, prNum, issueListCommentsOpts)
			//saveResponse(resp.Header, fmt.Sprintf("comments-header-%d.json", i))
			//respData, _ := io.ReadAll(resp.Body)
			//saveResponse(respData, fmt.Sprintf("comments-body-%d.json", i))
		}
		if err != nil {
			log.Error().Err(err).Msgf("Unable to fetch PR Comments %s/%s#%d", owner, repo, prNum)
			return nil, err
		}
		log.Debug().Msgf("Checking %d comments in %s/%s#%d", len(comments), owner, repo, prNum)
		for _, c := range comments {
			if *c.User.Login == commentLogin && strings.Contains(*c.Body, commentIdentifier) {
				res = append(res, c)
			}
		}
		if resp.NextPage > 0 {
			issueListCommentsOpts.Page = resp.NextPage
			checkComments = true
		}
	}
	return res, nil
}

// Creates or updates comment on the specified pull request
func Comment(ctx context.Context, owner, repo string, prNum int, sha string, commentBodies []string) ([]*github.IssueComment, error) {
	var res []*github.IssueComment
	if !isPrHead(ctx, sha, owner, repo, prNum) {
		log.Info().Msgf("%s is not HEAD for %s/%s#%d - skipping comment", sha, owner, repo, prNum)
		return res, nil
	}
	existingComments, err := getExistingComments(ctx, owner, repo, prNum)
	if err != nil {
		return res, err
	}
	nextExistingCommentIdx := 0
	for i, commentBody := range commentBodies {
		commentBody += "\n\n"
		commentBody += commentIdentifier
		commentBody += "\n"
		newComment := github.IssueComment{Body: &commentBody}
		var existingComment *github.IssueComment
		var issueComment *github.IssueComment
		var resp *github.Response
		if i < len(existingComments) {
			nextExistingCommentIdx = i + 1
			existingComment = existingComments[i]
			issueComment, resp, err = commentClient.Issues.EditComment(ctx, owner, repo, *existingComment.ID, &newComment)
		} else {
			issueComment, resp, err = commentClient.Issues.CreateComment(ctx, owner, repo, prNum, &newComment)
		}
		if resp != nil {
			log.Info().Msgf("%s received from %s", resp.Status, resp.Request.URL.String())
		}
		if err != nil {
			if existingComment != nil {
				log.Error().Err(err).Msgf("Failed to update comment %d for %s/%s#%d", *existingComment.ID, owner, repo, prNum)
			} else {
				log.Error().Err(err).Msgf("Failed to create comment for %s/%s#%d", owner, repo, prNum)
			}
			return res, err
		}
		if issueComment == nil {
			log.Error().Msg("issueComment is nil? How did I get here?")
			return res, fmt.Errorf("unknown error - issueComment is nil")
		}
		log.Info().Msgf("Created or Updated comment ID %d in %s/%s#%d: %s", *issueComment.ID, owner, repo, prNum, *issueComment.IssueURL)
		res = append(res, issueComment)
	}
	for nextExistingCommentIdx < len(existingComments) {
		existingComment := existingComments[nextExistingCommentIdx]
		truncateCommentBody := "[Outdated argo-diff content]\n\n" + commentIdentifier + "\n"
		newComment := github.IssueComment{Body: &truncateCommentBody}
		issueComment, resp, err := commentClient.Issues.EditComment(ctx, owner, repo, *existingComment.ID, &newComment)
		if resp != nil {
			log.Info().Msgf("%s received from %s", resp.Status, resp.Request.URL.String())
		}
		if err != nil {
			log.Error().Err(err).Msgf("Failed to update comment %d for %s/%s#%d", *existingComment.ID, owner, repo, prNum)
		} else {
			res = append(res, issueComment)
		}
		nextExistingCommentIdx++
	}
	return res, nil
}
