package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"

	"github.com/google/go-github/v41/github" // Ensure to get the latest version
	"golang.org/x/oauth2"
)

const secret = "YOUR_GITHUB_WEBHOOK_SECRET"
const token = "YOUR_GITHUB_API_TOKEN"
const commandToRun = "YOUR_COMMAND_HERE"

func verifySignature(payload []byte, signature string) bool {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := mac.Sum(nil)
	expectedSignature := "sha1=" + hex.EncodeToString(expectedMAC)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get("X-Hub-Signature")
	if !verifySignature(payload, signature) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	if event == "pull_request" {
		var prEvent github.PullRequestEvent
		if err := json.Unmarshal(payload, &prEvent); err != nil {
			http.Error(w, "Error unmarshalling JSON", http.StatusInternalServerError)
			return
		}

		if *prEvent.Action == "opened" || *prEvent.Action == "synchronize" {
			cmd := exec.Command("/bin/sh", "-c", commandToRun)
			var out bytes.Buffer
			cmd.Stdout = &out
			err := cmd.Run()

			// Setting up GitHub client
			ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
			tc := oauth2.NewClient(r.Context(), ts)
			client := github.NewClient(tc)

			// Determine command status and post commit status to GitHub
			var status string
			var description string
			if err != nil {
				status = "failure"
				description = "Command execution failed."
			} else {
				status = "success"
				description = "Command executed successfully."
			}

			repoStatus := &github.RepoStatus{State: &status, Description: &description, Context: github.String("continuous-integration/my-ci")}
			_, _, err = client.Repositories.CreateStatus(r.Context(), *prEvent.Repo.Owner.Login, *prEvent.Repo.Name, *prEvent.PullRequest.Head.SHA, repoStatus)
			if err != nil {
				http.Error(w, "Error setting commit status on GitHub", http.StatusInternalServerError)
				return
			}

			comment := &github.IssueComment{Body: github.String(out.String())}
			_, _, err = client.Issues.CreateComment(r.Context(), *prEvent.Repo.Owner.Login, *prEvent.Repo.Name, *prEvent.PullRequest.Number, comment)
			if err != nil {
				http.Error(w, "Error creating comment on GitHub", http.StatusInternalServerError)
				return
			}
		}
	}
}

func printWebHook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get("X-Hub-Signature")
	if !verifySignature(payload, signature) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	fmt.Sprintln("EVENT [%s]: %s", event, payload)
	return
}

func main() {
	//http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/webhook", printWebHook)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
