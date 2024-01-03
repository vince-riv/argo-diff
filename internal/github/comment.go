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

// Populates commentUser singleton with the Github user associated with our github client
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
			if *c.User.Login == *commentUser.Login && strings.Contains(*c.Body, commentIdentifier) {
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
func Comment(ctx context.Context, owner, repo string, prNum int, commentBodies []string) ([]*github.IssueComment, error) {
	var res []*github.IssueComment
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
