package github

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-github/v92/github"
	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/comment"
	"github.com/vince-riv/argo-diff/internal/scm"
)

// Provider implements argo-diff's provider primitives on top of the GitHub
// API. It holds no state: the clients are package vars built in init(), which
// tests swap for httptest-backed ones.
type Provider struct{}

var _ scm.Provider = Provider{}

// providerName is GitHub's name in the scm registry.
const providerName = "github"

func (Provider) Name() string { return providerName }

func (Provider) WebhookHandler() scm.WebhookHandler { return WebhookHandler{} }

// appEnvVars are the three variables a GitHub App installation needs.
var appEnvVars = []string{"GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY"}

// Enabled reports whether any GitHub credential is set: a token, or any of
// the GitHub App variables. ValidateConfig() catches an incomplete App setup.
func (Provider) Enabled() bool {
	if os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" {
		return true
	}
	for _, e := range appEnvVars {
		if os.Getenv(e) != "" {
			return true
		}
	}
	return false
}

// ValidateConfig requires all three GitHub App variables when no token is
// set.
func (Provider) ValidateConfig() error {
	if os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" {
		return nil
	}
	log.Info().Msg("GITHUB_PERSONAL_ACCESS_TOKEN or GITHUB_TOKEN environment variable not set - assuming Github App installation")
	for _, e := range appEnvVars {
		if os.Getenv(e) == "" {
			return fmt.Errorf("%s environment variable is not set for Github App installations", e)
		}
	}
	return nil
}

// CredentialsHint names the variables that enable this provider.
func (Provider) CredentialsHint() string {
	return "GITHUB_PERSONAL_ACCESS_TOKEN, GITHUB_TOKEN, or GITHUB_APP_ID + GITHUB_APP_INSTALLATION_ID + GITHUB_APP_PRIVATE_KEY"
}

func (Provider) ConnectivityCheck() error { return ConnectivityCheck() }

// RepoHosts is where GitHub repositories live, for matching ArgoCD
// application sources. GitHub Enterprise hosts match through argocd's
// host-agnostic owner/repo fallback.
func (Provider) RepoHosts() []string { return []string{"github.com"} }

func (Provider) Dialect() comment.Dialect { return comment.GitHub }

func (Provider) ListChangedFiles(ctx context.Context, repo scm.RepoRef, num int) ([]string, error) {
	return ListPullRequestFiles(ctx, repo.Owner, repo.Name, num)
}

// SetStatus is a no-op under GitHub Actions; see Status().
func (Provider) SetStatus(ctx context.Context, repo scm.RepoRef, sha string, state scm.Status, description string, dryRun bool) error {
	return Status(ctx, state, description, repo.Owner, repo.Name, sha, dryRun)
}

func (Provider) GetChangeRequest(ctx context.Context, repo scm.RepoRef, num int) (scm.ChangeRequest, error) {
	return GetPullRequest(ctx, repo.Owner, repo.Name, num)
}

// CurrentUser returns the comment author's login. Under GitHub Actions, or
// with the github connectivity check bypassed, it returns "" so comments are
// matched by marker alone: the token's identity can't be resolved there.
func (Provider) CurrentUser(ctx context.Context) (string, error) {
	if isGithubAction || bypassGithubCheck() {
		return "", nil
	}
	if err := getCommentUser(ctx); err != nil {
		return "", err
	}
	// an empty login means "match any author" to scm.ExistingComments, which
	// would let argo-diff edit other users' marker-bearing comments. Outside
	// the two marker-only cases above that is never what we want.
	login := getCommentLogin()
	if login == "" {
		return "", fmt.Errorf("github comment user resolved to an empty login")
	}
	return login, nil
}

// ListComments returns every comment on the pull request, oldest first.
func (Provider) ListComments(ctx context.Context, repo scm.RepoRef, num int) ([]scm.Comment, error) {
	if commentClient == nil {
		log.Error().Msg("Cannot call github API - I don't have a client set")
		return nil, fmt.Errorf("no github commenter client")
	}
	sortOpt := "created"
	sortDirection := "asc"
	opts := github.IssueListCommentsOptions{
		Sort:      &sortOpt,
		Direction: &sortDirection,
	}
	var res []scm.Comment
	for {
		comments, resp, err := commentClient.Issues.ListComments(ctx, repo.Owner, repo.Name, num, &opts)
		if resp != nil {
			log.Info().Msgf("%s received when calling commentClient.PullRequest.ListComments(%s, %s, %d, %v) via go-github", resp.Status, repo.Owner, repo.Name, num, opts)
		}
		if err != nil {
			log.Error().Err(err).Msgf("Unable to fetch PR Comments %s/%s#%d", repo.Owner, repo.Name, num)
			return nil, err
		}
		for _, c := range comments {
			res = append(res, toComment(c))
		}
		if resp.NextPage == 0 {
			return res, nil
		}
		opts.Page = resp.NextPage
	}
}

func (Provider) CreateComment(ctx context.Context, repo scm.RepoRef, num int, body string) (scm.Comment, error) {
	if commentClient == nil {
		return scm.Comment{}, fmt.Errorf("no github commenter client")
	}
	c, resp, err := commentClient.Issues.CreateComment(ctx, repo.Owner, repo.Name, num, github.IssueCommentRequest{Body: body})
	return postedComment(c, resp, err)
}

// UpdateComment edits an issue comment. GitHub addresses issue comments by ID
// alone, so num is unused.
func (Provider) UpdateComment(ctx context.Context, repo scm.RepoRef, num int, id int64, body string) (scm.Comment, error) {
	if commentClient == nil {
		return scm.Comment{}, fmt.Errorf("no github commenter client")
	}
	c, resp, err := commentClient.Issues.UpdateComment(ctx, repo.Owner, repo.Name, id, github.IssueCommentRequest{Body: body})
	return postedComment(c, resp, err)
}

// postedComment logs a create/update response and converts its comment.
func postedComment(c *github.IssueComment, resp *github.Response, err error) (scm.Comment, error) {
	if resp != nil {
		log.Info().Msgf("%s received from %s", resp.Status, resp.Request.URL.String())
	}
	if err != nil {
		return scm.Comment{}, err
	}
	if c == nil {
		log.Error().Msg("issueComment is nil? How did I get here?")
		return scm.Comment{}, fmt.Errorf("unknown error - issueComment is nil")
	}
	return toComment(c), nil
}

func toComment(c *github.IssueComment) scm.Comment {
	return scm.Comment{
		ID:     c.GetID(),
		Body:   c.GetBody(),
		Author: c.GetUser().GetLogin(),
		URL:    c.GetIssueURL(),
	}
}
