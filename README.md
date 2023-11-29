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
