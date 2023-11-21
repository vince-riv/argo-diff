#!/usr/bin/env bash

ns=argocd

cd "$(dirname "$0")"

read -p 'Github Webhook Secret: ' webhook_secret
read -p 'Github API Token: ' api_token
read -p 'ArgoCD Auth Token: ' argo_token

kubectl -n $ns create secret generic argo-diff-env \
    "--from-literal=GITHUB_WEBHOOK_SECRET=${webhook_secret}" \
    "--from-literal=GITHUB_PERSONAL_ACCESS_TOKEN=${api_token}" \
    "--from-literal=ARGOCD_AUTH_TOKEN=${argo_token}" \
    --dry-run=client -o yaml | kubeseal -n $ns -o yaml > env-secret.yaml
