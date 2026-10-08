package scm

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/vince-riv/argo-diff/internal/comment"
)

const headSha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// fakeCommenter is an in-memory Commenter that records the calls made to it.
type fakeCommenter struct {
	head      string
	headErr   error
	login     string
	userErr   error
	comments  []Comment
	nextID    int64
	updateErr map[int64]error
	created   []string
	updated   map[int64]string
}

func newFake(login string, comments ...Comment) *fakeCommenter {
	return &fakeCommenter{head: headSha, login: login, comments: comments, nextID: 9000, updated: map[int64]string{}}
}

func (f *fakeCommenter) GetChangeRequest(context.Context, RepoRef, int) (ChangeRequest, error) {
	return ChangeRequest{HeadSHA: f.head}, f.headErr
}

func (f *fakeCommenter) ListComments(context.Context, RepoRef, int) ([]Comment, error) {
	return f.comments, nil
}

func (f *fakeCommenter) CreateComment(_ context.Context, _ RepoRef, _ int, body string) (Comment, error) {
	f.nextID++
	f.created = append(f.created, body)
	return Comment{ID: f.nextID, Body: body}, nil
}

func (f *fakeCommenter) UpdateComment(_ context.Context, _ RepoRef, _ int, id int64, body string) (Comment, error) {
	if err := f.updateErr[id]; err != nil {
		return Comment{}, err
	}
	f.updated[id] = body
	return Comment{ID: id, Body: body}, nil
}

func (f *fakeCommenter) CurrentUser(context.Context) (string, error) {
	return f.login, f.userErr
}

var repo = RepoRef{Owner: "vince-riv", Name: "argo-diff"}

// ours is a comment a previous argo-diff run posted as author.
func ours(id int64, author string) Comment {
	return Comment{ID: id, Author: author, Body: comment.Wrap("old diff")}
}

func ids(cs []Comment) []int64 {
	var res []int64
	for _, c := range cs {
		res = append(res, c.ID)
	}
	return res
}

func TestExistingCommentsMatching(t *testing.T) {
	theirs := Comment{ID: 2, Author: "someone", Body: "looks good"}
	impostor := ours(3, "someone") // carries the marker, but another author
	f := newFake("argo-bot", ours(1, "argo-bot"), theirs, impostor, ours(4, "argo-bot"))

	got, err := ExistingComments(context.Background(), f, repo, 1)
	if err != nil {
		t.Fatalf("ExistingComments() err'd: %v", err)
	}
	if want := []int64{1, 4}; !slices.Equal(ids(got), want) {
		t.Errorf("ExistingComments() = %v, want %v (marker and author must both match)", ids(got), want)
	}

	// an empty login means the identity is unknown: match by marker alone
	f.login = ""
	got, _ = ExistingComments(context.Background(), f, repo, 1)
	if want := []int64{1, 3, 4}; !slices.Equal(ids(got), want) {
		t.Errorf("ExistingComments() with no login = %v, want %v", ids(got), want)
	}

	f.userErr = errors.New("401")
	if _, err := ExistingComments(context.Background(), f, repo, 1); err == nil {
		t.Error("ExistingComments() swallowed a CurrentUser() error")
	}
}

func TestPostCommentsReusesInOrder(t *testing.T) {
	f := newFake("argo-bot", ours(1, "argo-bot"), ours(2, "argo-bot"))

	// one body: edit comment 1, outdate comment 2
	got, err := PostComments(context.Background(), f, repo, 1, headSha, []string{"new diff"})
	if err != nil {
		t.Fatalf("PostComments() err'd: %v", err)
	}
	if want := []int64{1, 2}; !slices.Equal(ids(got), want) {
		t.Fatalf("PostComments() = %v, want %v", ids(got), want)
	}
	if f.updated[1] != comment.Wrap("new diff") {
		t.Errorf("comment 1 = %q, want the wrapped new body", f.updated[1])
	}
	if !strings.HasPrefix(f.updated[2], outdatedBody) || !strings.Contains(f.updated[2], comment.Identifier()) {
		t.Errorf("comment 2 = %q, want it outdated and still carrying the marker", f.updated[2])
	}
	if len(f.created) != 0 {
		t.Errorf("created %d comments, want none", len(f.created))
	}

	// three bodies over two existing comments: edit both, create one
	f = newFake("argo-bot", ours(1, "argo-bot"), ours(2, "argo-bot"))
	got, _ = PostComments(context.Background(), f, repo, 1, headSha, []string{"a", "b", "c"})
	if want := []int64{1, 2, 9001}; !slices.Equal(ids(got), want) {
		t.Errorf("PostComments() = %v, want %v", ids(got), want)
	}
	if len(f.created) != 1 || f.created[0] != comment.Wrap("c") {
		t.Errorf("created %q, want the third body only", f.created)
	}

	// no bodies: outdate everything
	f = newFake("argo-bot", ours(1, "argo-bot"), ours(2, "argo-bot"))
	_, _ = PostComments(context.Background(), f, repo, 1, headSha, nil)
	for _, id := range []int64{1, 2} {
		if !strings.HasPrefix(f.updated[id], outdatedBody) {
			t.Errorf("comment %d = %q, want it outdated", id, f.updated[id])
		}
	}
}

func TestPostCommentsHeadCheck(t *testing.T) {
	f := newFake("argo-bot")
	got, err := PostComments(context.Background(), f, repo, 1, "1111111111111111111111111111111111111111", []string{"x"})
	if err != nil || len(got) != 0 || len(f.created) != 0 {
		t.Errorf("PostComments() on a stale sha = %v, %v; created %d - want a silent no-op", got, err, len(f.created))
	}

	// a missing head SHA counts as "not HEAD"
	f.head = ""
	_, _ = PostComments(context.Background(), f, repo, 1, headSha, []string{"x"})
	if len(f.created) != 0 {
		t.Error("PostComments() posted although the change request has no head SHA")
	}

	// a lookup error assumes HEAD, so a flaky API doesn't suppress the comment
	f = newFake("argo-bot")
	f.headErr = errors.New("502")
	_, _ = PostComments(context.Background(), f, repo, 1, headSha, []string{"x"})
	if len(f.created) != 1 {
		t.Error("PostComments() skipped the comment on a head lookup error")
	}
}

func TestPostCommentsUpdateErrors(t *testing.T) {
	// failing to edit a body comment aborts
	f := newFake("argo-bot", ours(1, "argo-bot"))
	f.updateErr = map[int64]error{1: errors.New("422")}
	if _, err := PostComments(context.Background(), f, repo, 1, headSha, []string{"a", "b"}); err == nil {
		t.Error("PostComments() swallowed an update error")
	}
	if len(f.created) != 0 {
		t.Error("PostComments() kept going after an update error")
	}

	// failing to outdate a leftover is logged and skipped
	f = newFake("argo-bot", ours(1, "argo-bot"), ours(2, "argo-bot"), ours(3, "argo-bot"))
	f.updateErr = map[int64]error{2: errors.New("422")}
	got, err := PostComments(context.Background(), f, repo, 1, headSha, nil)
	if err != nil {
		t.Errorf("PostComments() err'd on an outdate failure: %v", err)
	}
	if want := []int64{1, 3}; !slices.Equal(ids(got), want) {
		t.Errorf("PostComments() = %v, want %v", ids(got), want)
	}
}
