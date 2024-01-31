#!/usr/bin/env bash

ns=argocd

cd "$(dirname "$0")"

read -p 'ArgoCD Auth Token: ' argo_token
read -p 'Github Webhook Secret: ' webhook_secret
read -p 'Github Personal Access Token: ' api_token
read -p 'Github App Private Key .pem file: ' app_private_key

secret_args=()

if [[ -n $argo_token ]]; then
    secret_args+=("--from-literal=ARGOCD_AUTH_TOKEN=${argo_token}")
fi
if [[ -n $webhook_secret ]]; then
    secret_args+=("--from-literal=GITHUB_PERSONAL_ACCESS_TOKEN=${api_token}")
fi
if [[ -n $api_token ]]; then
    secret_args+=("--from-literal=GITHUB_WEBHOOK_SECRET=${webhook_secret}")
fi
if [[ -n $app_private_key ]]; then
    secret_args+=("--from-file=GITHUB_APP_PRIVATE_KEY=${app_private_key}")
fi

kubectl -n $ns create secret generic argo-diff-env \
    ${secret_args[@]} \
    --dry-run=client -o yaml | kubeseal -n $ns -o yaml --merge-into env-secret.yaml
