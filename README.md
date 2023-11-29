# Argo-Diff

Application written in [go](https://go.dev/) that provides Github status checks and Pull Request comments
for changes to Kubernetes manifests when those manifests are delivered via
[ArgoCD](https://argo-cd.readthedocs.io/en/stable/).

## Overview

Argo-diff is designed to receive webhook notifications from Gihtub for `push` and `pull_request` events. When
events are received, it queries the ArgoCD API to pull manifests for the Argo application(s) configured for
the repository that is the source of the push or pull request. It will pull manifests both for the base ref
(eg: `main` branch) and the revision of the change.

If ArgoCD cannot generate manifests for the revision of the change, argo-diff will set its status check to a
failure for that associated commit.

If the event is for a pull request, argo-diff will comment on the associated pull request with markdown
displaying the diff of the manifests.

## Deploying

- Generate a fine-grained Github Personal Access Token. It should have the following Repository permissions:
  - *Administration*: `Read-only`
  - *Commit statuses*: `Read and write`
  - *Metadata*: `Read-only`
  - *Pull requests*: `Read and write`
- Create a user in your ArgoCD instance. This user should have read-only access to all applications:
  - For example, in _policy.csv_: `g, argo-diff, role:ci` and `p, role:ci, applications, get, *, allow`
  - This user shouldn't need a password but does need an API token to be generated.
- Generate a webhook secret that will be shared both by the argo-diff deployment and Github webhook config.
- Using the example manifests in the `docs/k8s/` directory, deploy argo-diff to the argocd namespace of your
    Kubernetes cluster. An Ingress or IngressRoute will need to be added to allow webhooks in from Github to
    the `/webhook` endpoint on the argo-diff Service.
- Configure organizational (or perhaps just repository level?) webhook notifications to argo-diff. The Payload
    URL should map the ingress configured in your cluster, and the secret should be the webhook secret
    previously generated. Invididual event types to configure:
  - *Issue comments* (for future use)
  - *Pull requests*
  - *Pushes*
- After the webhook is activated, the ping event should be received and verified by argo-diff and this will
    validate connectivity from Github to argo-diff

## Limitations

This is still in a proof-of-concept and alpha version state, so there are a number of known limitations.

- When fetching the list of Argo applications from the ArgoCD api, argo-diff currently doesn't handle
    pagination. So if you have a large number of applications, only the first chunk of them will currently be
    recognized by argo-diff.
- If there's a problem, and the diff comment needs to be regenerated, an admin must redeliver the webhook
    event associated with the PR. In the future, argo-diff may be re-initated by a comment on the PR.
- The list of Argo applications are cached in-memory for 15min. This means there's an opportunity for a race
    condition for when an Argo application has its configuration changed (git URL and/or name being the
    critical ones to argo-diff) argo-diff may not behave as expected against the app for up to 15 minutes.
- When many Argo applications are served by a single repository, performance is slow. Manifests for each Argo
    application are fetched sequentially, so this could result in argo-diff statuses and/or comments taking
    minutes to complete.

## Running locally

Set environment variables used by argo-diff and then execute `go run cmd/main.go`.

For example, you can place these in a file called `.env.sh`:

```sh
GITHUB_PERSONAL_ACCESS_TOKEN='github_pat_XXXX'
ARGOCD_AUTH_TOKEN='ABCDEFGHIJKLMNOPQRSTUVWXYZ'
ARGOCD_BASE_URL='https://argocd.your.domain'
ARGOCD_UI_BASE_URL='https://argocd.your.domain'
APP_ENV='dev'
```

Then source it and execute go run:

```
$ set -o allexport ; . .env.sh ; set +o allexport
$ go run cmd/main.go
```

To send requests, you can copy webhook request headers and payloads to `temp/curl-headers.txt` and
`temp/curl-payload.json` and use the `post-local.sh` script to ship them to the local server.

There is also the `/dev` endpoint that gets enabled when `APP_ENV=dev` - this endpoint is handled by
`devHandler()` in main.go and can be hardcoded to specific event data.

## Development Notes

This was originally developed by @vrivellino as a way to learn Go. Its functionality replicates that of an
internal tool written by smart people at a previous job.

For contributions, please use [gofmt](https://pkg.go.dev/cmd/gofmt) and the following linters: errcheck,
gosimple, govet, ineffassign, staticcheck, unused.
