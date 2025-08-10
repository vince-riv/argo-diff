# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Argo-diff is a Go application that provides Pull Request comments for changes to Kubernetes manifests delivered via ArgoCD. It can run as a webhook server or through GitHub Actions to automatically detect changes in ArgoCD applications and generate diffs against the live cluster state.

## Development Commands

### Build and Run
```bash
# Build the application
go build -v ./...

# Run locally (requires environment variables)
go run cmd/main.go

# Run with a specific event file
go run cmd/main.go -f path/to/event_info.json

# Format code
go fmt ./...

# Run tests
go test -v ./...
```

### Environment Setup
Create a `.env.sh` file for local development:
```bash
GITHUB_PERSONAL_ACCESS_TOKEN='github_pat_XXXX'
ARGOCD_AUTH_TOKEN='YOUR_ARGOCD_TOKEN'
ARGOCD_SERVER_ADDR='argocd.your.domain:443'
ARGOCD_UI_BASE_URL='https://argocd.your.domain'
APP_ENV='dev'
```

Load environment and run:
```bash
set -o allexport ; . .env.sh ; set +o allexport
go run cmd/main.go
```

## Architecture

### Core Packages

- **cmd/main.go**: Application entry point with CLI argument parsing and environment validation
- **internal/webhook/**: Handles GitHub webhook event processing and EventInfo struct definitions
- **internal/argocd/**: ArgoCD API client, connectivity checks, and manifest processing
- **internal/github/**: GitHub API integration, comment generation, and status checks
- **internal/server/**: HTTP server implementation for webhook processing and GitHub Actions mode
- **internal/process_event/**: Core business logic for processing code changes and generating diffs
- **internal/gendiff/**: Diff generation utilities

### Key Data Structures

- **EventInfo** (internal/webhook/process.go): Core event data structure containing repository info, PR details, and changed files
- **ApplicationResourcesWithChanges** (internal/argocd/types.go): Contains ArgoCD application with changed resources and diffs
- **AppResource** (internal/argocd/types.go): Individual Kubernetes resource with diff information

### Operational Modes

1. **Webhook Server Mode**: Receives GitHub webhook events and processes them
2. **GitHub Actions Mode**: Runs once when `GITHUB_ACTIONS=true` environment variable is set
3. **File Processing Mode**: Processes a single event from a JSON file using `-f` flag
4. **Dev Mode**: Enabled with `APP_ENV=dev`, provides `/dev` endpoint for manual testing

### Required Environment Variables

**Critical (Application will fail without these):**
- `ARGOCD_AUTH_TOKEN`: Bearer token for ArgoCD API access
- `ARGOCD_SERVER_ADDR`: ArgoCD server address
- `GITHUB_WEBHOOK_SECRET`: For webhook mode only
- GitHub authentication (one of):
  - `GITHUB_TOKEN` or `GITHUB_PERSONAL_ACCESS_TOKEN`
  - Or GitHub App credentials: `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PRIVATE_KEY`

## Testing

The codebase includes comprehensive test coverage with test data in `*_testdata/` directories:
- `internal/argocd/argocd_testdata/`: ArgoCD API response fixtures
- `internal/github/github_testdata/`: GitHub API response fixtures  
- `internal/webhook/webhook_testdata/`: GitHub webhook payload samples

Tests can be run with standard Go testing commands and use real API response data for reliable testing.

## GitHub Actions Integration

The project includes GitHub Actions workflows:
- **go.yml**: Runs build, format, lint (golangci-lint v1.61), and tests
- **dockerbuild.yml**: Builds and publishes Docker images
- **helm.yml**: Manages Helm chart releases
- **chart-releaser.yml**: Handles chart version releases

## Local Development

### Testing HTTP Endpoints
- Use `post-local.sh` script with `temp/curl-headers.txt` and `temp/curl-payload.json`
- Dev endpoint at `/dev` when `APP_ENV=dev` (bypasses webhook processing)
- Dev mode disables status checks but allows commenting

### Event File Format
Create JSON file matching `EventInfo` struct for local testing:
```json
{
    "ignore": false,
    "owner": "GITHUB_ORG_NAME",
    "repo": "REPOSITORY_NAME", 
    "default_ref": "main",
    "commit_sha": "LONG_SHA_OF_COMMIT",
    "pr": 123,
    "change_ref": "BRANCH_NAME_OF_PR",
    "base_ref": "main"
}
```

## Deployment Options

1. **Helm Chart**: `helm install my-release oci://ghcr.io/vince-riv/chart/argo-diff`
2. **Kubernetes Manifests**: Use examples in `docs/k8s/`
3. **GitHub Actions**: Use `vince-riv/argo-diff@actions-v1` action
4. **Docker**: Multi-stage Dockerfiles available (`Dockerfile`, `Dockerfile.actions`, `Dockerfile.no-build`)