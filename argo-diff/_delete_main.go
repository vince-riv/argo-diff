package main

import (
	"io"
	"net/http"

	"argo-diff/webhook"

	// Ensure to get the latest version

	"github.com/rs/zerolog/log"
)

var (
	githubWebhookSecret string
	githubApiToken      string
)

const commandToRun = "YOUR_COMMAND_HERE"

const gitRevTxt = "git-rev.txt"

const sigHeaderName = "X-Hub-Signature-256"

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading request body")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get(sigHeaderName)
	if !webhook.VerifySignature(payload, signature, githubWebhookSecret) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	switch event {
	case "ping":
		log.Info().Str("method", r.Method).Str("url", r.URL.String()).Msg("ping event received")
		_, err := io.WriteString(w, "event processed\n")
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	case "pull_request":
		pr, repo, err := webhook.ProcessPullRequest(payload)
		if err != nil {
			http.Error(w, "Could not process pull request event data", http.StatusInternalServerError)
			return
		}
		if (pr == nil && repo != nil) || (pr != nil && repo == nil) {
			log.Error().Msg("Unexpected result processing pull request event")
			http.Error(w, "Unexpected result processing pull request event", http.StatusInternalServerError)
			return
		}
		if pr == nil {
			_, err := io.WriteString(w, "event processed\n")
			if err != nil {
				log.Error().Err(err).Msg("io.WriteString() failed")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
		// 1) get ArgoCD projects
		// 2) build list of matching based on repo and (target) branch
		// 3) call argo diff against the revision
		// 4) update commit status
		// 5) comment (or update comment) in pull request
	case "push":
		// PUSH
		headCommit, repo, err := webhook.ProcessPush(payload)
		if err != nil {
			http.Error(w, "Could not process push event data", http.StatusInternalServerError)
			return
		}
		if (headCommit == nil && repo != nil) || (headCommit != nil && repo == nil) {
			log.Error().Msg("Unexpected result processing push event")
			http.Error(w, "Unexpected result processing push event", http.StatusInternalServerError)
			return
		}
		if headCommit == nil {
			_, err := io.WriteString(w, "event processed\n")
			if err != nil {
				log.Error().Err(err).Msg("io.WriteString() failed")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
		// 0) check branch regex?
		// 1) get ArgoCD projects
		// 2) build list of matching based on repo and branch
		// 3) call argo diff against the revision
		// 4) update commit status
	default:
		log.Info().Str("method", r.Method).Str("url", r.URL.String()).Msgf("Ignoring X-GitHub-Event %s", event)
		_, err := io.WriteString(w, "event ignored\n")
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// cmd := exec.command("/bin/sh", "-c", commandtorun)
	// var out bytes.buffer
	// cmd.stdout = &out
	// err := cmd.run()
	//
	// // setting up github client
	// ts := oauth2.statictokensource(&oauth2.token{accesstoken: githubapitoken})
	// tc := oauth2.newclient(r.context(), ts)
	// client := github.newclient(tc)
	//
	// // determine command status and post commit status to github
	// var status string
	// var description string
	//
	//	if err != nil {
	//		status = "failure"
	//		description = "command execution failed."
	//	} else {
	//
	//		status = "success"
	//		description = "command executed successfully."
	//	}
	//
	// repostatus := &github.repostatus{state: &status, description: &description, context: github.string("continuous-integration/my-ci")}
	// _, _, err = client.repositories.createstatus(r.context(), *prevent.repo.owner.login, *prevent.repo.name, *prevent.pullrequest.head.sha, repostatus)
	//
	//	if err != nil {
	//		http.error(w, "error setting commit status on github", http.statusinternalservererror)
	//		return
	//	}
	//
	// comment := &github.issuecomment{body: github.string(out.string())}
	// _, _, err = client.issues.createcomment(r.context(), *prevent.repo.owner.login, *prevent.repo.name, *prevent.pullrequest.number, comment)
	//
	//	if err != nil {
	//		http.error(w, "error creating comment on github", http.statusinternalservererror)
	//		return
	//	}
}

func healthz(w http.responsewriter, r *http.request) {
	//fmt.sprintln("event [%s]: %s", event, payload)
	log.debug().str("method", r.method).str("url", r.url.string()).msg("healthz endpoint")
	_, err := io.writestring(w, "healthy\n")
	if err != nil {
		http.error(w, "io.writestring() failed", http.statusinternalservererror)
		return
	}
}

func printwebhook(w http.responsewriter, r *http.request) {
	payload, err := io.readall(r.body)
	if err != nil {
		log.error().msg("failed to read request body")
		http.error(w, "error reading request body", http.statusinternalservererror)
		return
	}

	event := r.header.get("x-github-event")
	log.info().str("method", r.method).str("url", r.url.string()).str("event", event).msg(string(payload))

	signature := r.header.get(sigheadername)
	if !webhook.verifysignature(payload, signature, githubwebhooksecret) {
		log.warn().msg("invalid signature")
		http.error(w, "invalid signature", http.statusunauthorized)
		return
	}
}

func init() {
	// load github secrets from env and setup logger
	debug := true // todo: switch to env var
	gitrev := "unknown"
	zerolog.setgloballevel(zerolog.infolevel)
	if debug {
		zerolog.setgloballevel(zerolog.debuglevel)
	}

	data, err := os.readfile(gitrevtxt)
	if err != nil {
		log.info().msg(fmt.sprintf("cannot open %s; assuming we're in local development", gitrevtxt))
	} else {
		lines := strings.split(string(data), "\n")
		gitrev = strings.trimspace(lines[0])
		if gitrev == "" {
			log.warn().msg(fmt.sprintf("%s must be empty?", gitrevtxt))
			gitrev = "empty"
		}
	}

	log.logger = log.with().str("service", "argo-diff").str("version", gitrev).caller().logger()

	githubwebhooksecret = os.getenv("github_webhook_secret")
	if githubwebhooksecret == "" {
		log.fatal().msg("github_webhook_secret environment variable not set")
	}

	githubapitoken = os.getenv("github_api_token")
	if githubapitoken == "" {
		log.fatal().msg("github_api_token environment variable not set")
	}
}

func main() {
	log.info().msg("setting up listener on port 8080")
	//http.handlefunc("/webhook", handleWebhook)
	http.HandleFunc("/webhook", printWebHook)
	http.HandleFunc("/healthz", healthZ)
	log.Error().Err(http.ListenAndServe(":8080", nil)).Msg("")
}
