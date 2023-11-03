#!/usr/bin/env bash

ns=argocd

cd "$(dirname "$0")"

read -p 'Github Webhook Secret: ' webhook_secret
read -p 'Github API Token: ' api_token

kubectl -n $ns create secret generic argo-diff-env \
    "--from-literal=GITHUB_WEBHOOK_SECRET=${webhook_secret}" \
    "--from-literal=GITHUB_API_TOKEN=${api_token}" \
    --dry-run=client -o yaml | kubeseal -n $ns -o yaml > env-secret.yaml
