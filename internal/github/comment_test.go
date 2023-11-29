package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v56/github"
)

const testDataDir = "github_testdata"
const payloadUser = "payload-user.json"
const payloadPr1Comments = "payload-pr-1-comments.json"
const payloadPr2Comments = "payload-pr-2-comments.json"
const payloadPr3Comments = "payload-pr-3-comments.json"
const payloadPr1CreateComment = "payload-pr-1-create-comment.json"
const payloadPr2CreateComment = "payload-pr-2-create-comment.json"
const payloadPr3UpdateComment = "payload-pr-3-update-comment.json"

func readFileToByteArray(fileName string) ([]byte, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("Error getting current working directory: %w", err)
	}

	filePath := filepath.Join(workingDir, testDataDir, fileName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, filePath, fmt.Errorf("error reading file '%s': %w", filePath, err)
	}

	return data, filePath, nil
}

func newHttpTestServer(t *testing.T) *httptest.Server {
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCode := http.StatusNotFound
		payload := []byte(`404 Page Not Found`)
		var filePath string
		var err error
		//println("r.URL.Path: " + r.URL.Path)
		switch r.URL.Path {
		case "/user":
			statusCode = http.StatusOK
			payload, filePath, err = readFileToByteArray(payloadUser)
		case "/repos/vince-riv/argo-diff/issues/1/comments":
			if r.Method == "GET" {
				statusCode = http.StatusOK
				payload, filePath, err = readFileToByteArray(payloadPr1Comments)
			} else if r.Method == "POST" {
				statusCode = http.StatusCreated
				payload, filePath, err = readFileToByteArray(payloadPr1CreateComment)
			} else {
				statusCode = http.StatusMethodNotAllowed
			}
		case "/repos/vince-riv/argo-diff/issues/2/comments":
			if r.Method == "GET" {
				statusCode = http.StatusOK
				payload, filePath, err = readFileToByteArray(payloadPr2Comments)
			} else if r.Method == "POST" {
				statusCode = http.StatusCreated
				payload, filePath, err = readFileToByteArray(payloadPr2CreateComment)
			} else {
				statusCode = http.StatusMethodNotAllowed
			}
		case "/repos/vince-riv/argo-diff/issues/3/comments":
			statusCode = http.StatusOK
			payload, filePath, err = readFileToByteArray(payloadPr3Comments)
		case "/repos/vince-riv/argo-diff/issues/comments/3333333333":
			if r.Method == "PATCH" {
				statusCode = http.StatusOK
				payload, filePath, err = readFileToByteArray(payloadPr3UpdateComment)
			} else {
				statusCode = http.StatusMethodNotAllowed
			}
		default:
			t.Errorf("Mock server not configured to serve path %s", r.URL.Path)
		}
		if err != nil {
			t.Errorf("Failed to load %s: %s", filePath, err)
			statusCode = http.StatusInternalServerError
		}
		w.WriteHeader(statusCode)
		w.Write(payload)
	}))
	return newServer
}

func TestCommentNoExistingComments(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	httpBaseUrl, _ := url.Parse(server.URL + "/")
	commentClient = github.NewClient(nil).WithAuthToken("test1234")
	commentClient.BaseURL = httpBaseUrl
	commentClient.UploadURL = httpBaseUrl

	c, err := getExistingComment(context.Background(), "vince-riv", "argo-diff", 1)
	if err != nil {
		t.Errorf("getExistingComment() failed: %s", err)
	}
	if c != nil {
		t.Error("Expected no existing comment")
	}

	commentId, err := Comment(context.Background(), "vince-riv", "argo-diff", 1, "argo-diff test comment")
	if err != nil {
		t.Errorf("getExistingComment() failed: %s", err)
	}
	if commentId != 1111111111 {
		t.Errorf("Comment ID %d doesn't match 1111111111", commentId)
	}
}

func TestCommentExistingDifferentUser(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	httpBaseUrl, _ := url.Parse(server.URL + "/")
	commentClient = github.NewClient(nil).WithAuthToken("test1234")
	commentClient.BaseURL = httpBaseUrl
	commentClient.UploadURL = httpBaseUrl

	c, err := getExistingComment(context.Background(), "vince-riv", "argo-diff", 2)
	if err != nil {
		t.Errorf("getExistingComment() failed: %s", err)
	}
	if c != nil {
		t.Error("Expected no existing comment")
	}

	commentId, err := Comment(context.Background(), "vince-riv", "argo-diff", 2, "argo-diff test comment")
	if err != nil {
		t.Errorf("getExistingComment() failed: %s", err)
	}
	if commentId != 2222222222 {
		t.Errorf("Comment ID %d doesn't match 2222222222", commentId)
	}
}

func TestCommentExisting(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	httpBaseUrl, _ := url.Parse(server.URL + "/")
	commentClient = github.NewClient(nil).WithAuthToken("test1234")
	commentClient.BaseURL = httpBaseUrl
	commentClient.UploadURL = httpBaseUrl

	c, err := getExistingComment(context.Background(), "vince-riv", "argo-diff", 3)
	if err != nil {
		t.Errorf("getExistingComment() failed: %s", err)
	}
	if c == nil {
		t.Error("Could not find existing comment")
	} else if *c.ID != 3333333333 {
		t.Error("Expected issue comment ID to be 3333333333")
	}

	commentId, err := Comment(context.Background(), "vince-riv", "argo-diff", 3, "argo-diff test comment")
	if err != nil {
		t.Errorf("getExistingComment() failed: %s", err)
	}
	if commentId != 3333333333 {
		t.Errorf("Comment ID %d doesn't match 3333333333", commentId)
	}
}
