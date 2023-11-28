package github

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/google/go-github/v56/github"
	"github.com/rs/zerolog/log"
)

var (
	commentClient *github.Client
	commentUser   *github.User
	mux           *sync.RWMutex
)

const commentIdentifier = "<!-- comment produced by argo-diff -->"

func init() {
	githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	if githubPAT == "" {
		log.Error().Msg("Cannot create github client - GITHUB_PERSONAL_ACCESS_TOKEN is empty")
	} else {
		commentClient = github.NewClient(nil).WithAuthToken(githubPAT)
	}
	mux = &sync.RWMutex{}
}

func getCommentUser(ctx context.Context) error {
	if commentClient == nil {
		log.Error().Msg("Cannot call github API - I don't have a client set")
		return fmt.Errorf("no github commenter client")
	}
	mux.RLock()
	if commentUser != nil {
		mux.RUnlock()
		return nil
	}
	mux.RUnlock()
	user, resp, err := commentClient.Users.Get(ctx, "")
	if resp != nil {
		log.Info().Msgf("%s received when calling client.Users.Get() via go-github", resp.Status)
	}
	if err != nil {
		log.Error().Err(err).Msg("Unable to determine get my github user")
		return err
	}
	if user == nil {
		log.Error().Msg("Empty user returned - not sure how I got here")
		return nil
	}
	mux.Lock()
	commentUser = user
	mux.Unlock()
	return nil
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

func getExistingComment(ctx context.Context, owner, repo string, prNum int) (*github.IssueComment, error) {
	sortOpt := "created"
	sortDirection := "asc"
	issueListCommentsOpts := github.IssueListCommentsOptions{
		Sort:      &sortOpt,
		Direction: &sortDirection,
	}
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
			if *c.User.Login == *commentUser.Login && strings.Contains(*c.Body, commentIdentifier) {
				return c, nil
			}
		}
		if resp.NextPage > 0 {
			issueListCommentsOpts.Page = resp.NextPage
			checkComments = true
		}
	}
	return nil, nil
}

func Comment(ctx context.Context, owner, repo string, prNum int, commentBody string) (int64, error) {
	existingComment, err := getExistingComment(ctx, owner, repo, prNum)
	if err != nil {
		return -1, err
	}
	commentBody += "\n\n"
	commentBody += commentIdentifier
	commentBody += "\n"
	newComment := github.IssueComment{Body: &commentBody}
	var issueComment *github.IssueComment
	var resp *github.Response
	if existingComment != nil {
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
		return -1, err
	}
	if issueComment == nil {
		log.Error().Msg("issueComment is nil? How did I get here?")
		return -1, fmt.Errorf("unknown error - issueComment is nil")
	}
	log.Info().Msgf("Created or Updated comment ID %d in %s/%s#%d: %s", *issueComment.ID, owner, repo, prNum, *issueComment.IssueURL)
	return *issueComment.ID, nil
}
