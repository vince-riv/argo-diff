package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureDir = "../../internal/github/github_testdata/webhook"

// testSecret signs requests in the --verify tests. The github package reads
// GITHUB_WEBHOOK_SECRET at load time, so those tests re-run this test binary
// with it set (see TestVerifyHelper).
const testSecret = "webhook-parse-test-secret"

// rawRequest builds a raw HTTP request, as a receiver would write it to stdin.
func rawRequest(t *testing.T, headers map[string]string, body []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("POST /webhook HTTP/1.1\r\nHost: example.com\r\n")
	fmt.Fprintf(&b, "Content-Length: %d\r\n", len(body))
	for k, v := range headers {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	b.WriteString("\r\n")
	b.Write(body)
	return b.Bytes()
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func sign(body []byte) string {
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func runParse(t *testing.T, args []string, stdin []byte) (result, error) {
	t.Helper()
	var out bytes.Buffer
	err := run(args, bytes.NewReader(stdin), &out)
	var res result
	if out.Len() > 0 {
		if jerr := json.Unmarshal(out.Bytes(), &res); jerr != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", jerr, out.String())
		}
	}
	return res, err
}

func TestRunGitHub(t *testing.T) {
	tests := []struct {
		name, event, fixture string
		wantKind             string
		wantIgnore           bool
		wantRefresh          bool
	}{
		{"pr opened", "pull_request", "payload-pr-open.json", "change", false, false},
		{"pr synchronize", "pull_request", "payload-pr-sync.json", "change", false, false},
		{"pr closed", "pull_request", "payload-pr-close.json", "change", true, false},
		{"refresh comment", "issue_comment", "payload-comment-argodiff-created.json", "change", false, true},
		{"other comment", "issue_comment", "payload-comment-created.json", "change", true, false},
		{"ping", "ping", "payload-pr-open.json", "ping", false, false},
		{"unhandled event", "push", "payload-pr-open.json", "ignored", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := rawRequest(t, map[string]string{"X-GitHub-Event": tt.event}, fixture(t, tt.fixture))
			res, err := runParse(t, nil, req)
			if err != nil {
				t.Fatalf("run() error: %v", err)
			}
			if res.Provider != "github" {
				t.Errorf("got provider %q, want github", res.Provider)
			}
			if res.Name != tt.event {
				t.Errorf("got name %q, want %q", res.Name, tt.event)
			}
			if res.Kind != tt.wantKind {
				t.Errorf("got kind %q, want %q", res.Kind, tt.wantKind)
			}
			if res.Verified {
				t.Error("got verified without --verify")
			}
			if tt.wantKind != "change" {
				if res.Info != nil {
					t.Errorf("got info %+v for a %s event, want none", res.Info, tt.wantKind)
				}
				return
			}
			if res.Info == nil {
				t.Fatal("got no info for a change event")
			}
			if res.Info.Provider != "github" || res.Info.Ignore != tt.wantIgnore || res.Info.Refresh != tt.wantRefresh {
				t.Errorf("got info %+v", *res.Info)
			}
			if !tt.wantIgnore && (res.Info.RepoOwner == "" || res.Info.RepoName == "" || res.Info.PrNum <= 0) {
				t.Errorf("actionable event missing fields: %+v", *res.Info)
			}
		})
	}
}

func TestRunErrors(t *testing.T) {
	t.Run("unknown provider", func(t *testing.T) {
		if _, err := runParse(t, []string{"--provider", "bitbucket"}, nil); err == nil {
			t.Error("want an error")
		}
	})
	t.Run("not an HTTP request", func(t *testing.T) {
		if _, err := runParse(t, nil, []byte(`{"action":"opened"}`)); err == nil {
			t.Error("want an error")
		}
	})
	t.Run("malformed body", func(t *testing.T) {
		req := rawRequest(t, map[string]string{"X-GitHub-Event": "pull_request"}, []byte("{not json"))
		res, err := runParse(t, nil, req)
		if err == nil || !strings.HasPrefix(res.Error, "parse:") {
			t.Errorf("got err=%v result=%+v, want a parse error in both", err, res)
		}
		if res.Info != nil {
			t.Errorf("got info %+v for a failed parse, want none", *res.Info)
		}
	})
	t.Run("gitlab webhooks unsupported", func(t *testing.T) {
		req := rawRequest(t, map[string]string{"X-Gitlab-Event": "Merge Request Hook"}, []byte("{}"))
		res, err := runParse(t, []string{"--provider", "gitlab"}, req)
		if err == nil || res.Provider != "gitlab" || res.Name != "Merge Request Hook" {
			t.Errorf("got err=%v result=%+v", err, res)
		}
	})
}

// TestVerify re-runs this test binary as TestVerifyHelper with
// GITHUB_WEBHOOK_SECRET set (empty for "nosecret"), since the github package
// reads it at load time.
func TestVerify(t *testing.T) {
	for _, mode := range []string{"good", "bad", "nosecret"} {
		t.Run(mode, func(t *testing.T) {
			secret := testSecret
			if mode == "nosecret" {
				secret = ""
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestVerifyHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "GITHUB_WEBHOOK_SECRET="+secret, "WEBHOOK_PARSE_VERIFY_MODE="+mode)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("helper failed: %v\n%s", err, out)
			}
			// A renamed helper would match nothing and still exit 0.
			if !bytes.Contains(out, []byte("--- PASS: TestVerifyHelper")) {
				t.Errorf("helper did not run and pass:\n%s", out)
			}
		})
	}
}

func TestVerifyHelper(t *testing.T) {
	mode := os.Getenv("WEBHOOK_PARSE_VERIFY_MODE")
	if mode == "" {
		t.Skip("only run by TestVerify")
	}
	body := fixture(t, "payload-pr-open.json")
	sig := sign(body)
	if mode == "bad" {
		sig = sign([]byte("something else"))
	}
	req := rawRequest(t, map[string]string{"X-GitHub-Event": "pull_request", "X-Hub-Signature-256": sig}, body)
	res, err := runParse(t, []string{"--verify"}, req)
	switch mode {
	case "good":
		if err != nil || !res.Verified || res.Kind != "change" || res.Info == nil || res.Info.Ignore {
			t.Errorf("got err=%v result=%+v", err, res)
		}
	case "bad":
		if err == nil || res.Verified || res.Info != nil || res.Kind != "" || res.Error != "verify: invalid signature" {
			t.Errorf("got err=%v result=%+v", err, res)
		}
	case "nosecret":
		if err == nil || res.Verified || res.Info != nil || !strings.Contains(res.Error, "GITHUB_WEBHOOK_SECRET") {
			t.Errorf("got err=%v result=%+v, want an error naming the missing secret", err, res)
		}
	default:
		t.Fatalf("unknown mode %q", mode)
	}
}
