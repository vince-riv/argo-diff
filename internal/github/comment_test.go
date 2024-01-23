package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v58/github"
)

const testDataDir = "github_testdata"
const payloadUser = "payload-user.json"
const payloadPr1Comments = "payload-pr-1-comments.json"
const payloadPr2Comments = "payload-pr-2-comments.json"
const payloadPr3Comments = "payload-pr-3-comments.json"
const payloadPr4Comments = "payload-pr-4-comments.json"
const payloadPr1CreateComment = "payload-pr-1-create-comment.json"
const payloadPr2CreateComment = "payload-pr-2-create-comment.json"

// const payloadPr3UpdateComment = "payload-pr-3-update-comment.json"
const payloadPatchComment = "payload-pr-patch-comment.json"
const payloadPullRequest = "payload-pr-get.json"

const prHeadSha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

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

func jsonFieldExtract(srcField string, src []byte, destField string, dest []byte) ([]byte, error) {
	var srcData, destData map[string]interface{}
	err := json.Unmarshal(src, &srcData)
	if err != nil {
		return []byte{}, err
	}
	err = json.Unmarshal(dest, &destData)
	if err != nil {
		return []byte{}, err
	}
	destData[destField] = srcData[srcField]

	ret, err := json.Marshal(destData)
	if err != nil {
		return []byte{}, err
	}
	return ret, nil
}

func newHttpTestServer(t *testing.T) *httptest.Server {
	// TODO - unclutter this mess
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCode := http.StatusNotFound
		payload := []byte(`404 Page Not Found`)
		var filePath string
		var err error
		//println("r.URL.Path: " + r.URL.Path)
		if strings.HasPrefix(r.URL.Path, "/repos/vince-riv/argo-diff/issues/comments/") {
			if r.Method == "PATCH" {
				var reqBody []byte
				reqBody, err = io.ReadAll(r.Body)
				urlPathParts := strings.Split(r.URL.Path, "/")
				if urlPathParts[6] == "" {
					statusCode = http.StatusInternalServerError
					t.Errorf("Bad URL Path: %s", r.URL.Path)
				} else {
					statusCode = http.StatusOK
					payload, filePath, err = readFileToByteArray(payloadPatchComment)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						t.Errorf("readFileToByteArray() failed: %s", err)
						return
					}
					payload = bytes.Replace(payload, []byte("%%_COMMENT_ID_%%"), []byte(urlPathParts[6]), -1)
					payload, err = jsonFieldExtract("body", reqBody, "body", payload)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						t.Errorf("jsonFieldExtract() failed: %s", err)
						return
					}
				}
			} else {
				statusCode = http.StatusMethodNotAllowed
			}
		} else if strings.HasPrefix(r.URL.Path, "/repos/vince-riv/argo-diff/pulls/") {
			if r.Method == "GET" {
				urlPathParts := strings.Split(r.URL.Path, "/")
				if urlPathParts[5] == "" {
					statusCode = http.StatusInternalServerError
					t.Errorf("Bad URL Path: %s", r.URL.Path)
				} else {
					statusCode = http.StatusOK
					payload, filePath, err = readFileToByteArray(payloadPullRequest)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						t.Errorf("readFileToByteArray() failed: %s", err)
						return
					}
					payload = bytes.Replace(payload, []byte("%%_PR_NUM_%%"), []byte(urlPathParts[5]), -1)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						t.Errorf("jsonFieldExtract() failed: %s", err)
						return
					}
				}
			} else {
				statusCode = http.StatusMethodNotAllowed
			}
		} else {
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
			case "/repos/vince-riv/argo-diff/issues/4/comments":
				statusCode = http.StatusOK
				payload, filePath, err = readFileToByteArray(payloadPr4Comments)
			default:
				t.Errorf("Mock server not configured to serve path %s", r.URL.Path)
			}
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

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 1)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) > 0 {
		t.Error("Expected no existing comment")
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 1, prHeadSha, []string{"argo-diff test comment"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 1111111111 {
		t.Error("Comment ID doesn't match")
	}
}

func TestCommentExistingDifferentUser(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	httpBaseUrl, _ := url.Parse(server.URL + "/")
	commentClient = github.NewClient(nil).WithAuthToken("test1234")
	commentClient.BaseURL = httpBaseUrl
	commentClient.UploadURL = httpBaseUrl

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 2)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) > 0 {
		t.Error("Expected no existing comment")
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 2, prHeadSha, []string{"argo-diff test comment"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 2222222222 {
		t.Error("Comment ID doesn't match")
	}
}

func TestCommentExisting(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	httpBaseUrl, _ := url.Parse(server.URL + "/")
	commentClient = github.NewClient(nil).WithAuthToken("test1234")
	commentClient.BaseURL = httpBaseUrl
	commentClient.UploadURL = httpBaseUrl

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 3)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) != 1 {
		t.Error("Could not find existing comment")
	} else if *c[0].ID != 3333333333 {
		t.Error("Comment ID doesn't match")
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 3, prHeadSha, []string{"argo-diff test comment"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 3333333333 {
		t.Error("Comment ID doesn't match")
	}
}

func TestCommentExistingMulti(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	httpBaseUrl, _ := url.Parse(server.URL + "/")
	commentClient = github.NewClient(nil).WithAuthToken("test1234")
	commentClient.BaseURL = httpBaseUrl
	commentClient.UploadURL = httpBaseUrl

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 4)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) != 2 {
		t.Error("Could not find existing comment")
	} else if *c[0].ID != 4444444222 && *c[1].ID != 4444444333 {
		t.Error("Unexpected issue commit IDs")
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 4, prHeadSha, []string{"argo-diff test comment update"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 4444444222 {
		t.Errorf("1st Comment ID doesn't match 4444444222: %d", *comments[0].ID)
	}
	if !strings.Contains(*comments[0].Body, "argo-diff test comment update") {
		t.Errorf("1st Comment body doesn't match 'argo-diff test comment update': %s", *comments[0].Body)
	}
	if *comments[1].ID != 4444444333 {
		t.Errorf("2nd Comment ID doesn't match 4444444333: %d", *comments[1].ID)
	}
	if !strings.Contains(*comments[1].Body, "[Outdated argo-diff content]") {
		t.Errorf("1st Comment body doesn't match '[Outdated argo-diff content]': %s", *comments[1].Body)
	}
}

func TestCommentNotHead(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	httpBaseUrl, _ := url.Parse(server.URL + "/")
	commentClient = github.NewClient(nil).WithAuthToken("test1234")
	commentClient.BaseURL = httpBaseUrl
	commentClient.UploadURL = httpBaseUrl

	prHeadShaOld := "1111111111111111111111111111111111111111"

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 1, prHeadShaOld, []string{"argo-diff test comment"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if len(comments) > 0 {
		t.Error("Not expecting to comment")
	}
}
