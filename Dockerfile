ARG ARGOCD_IMAGE=quay.io/argoproj/argocd:v3.5.3@sha256:dd3f47d5a5e4da563a7a398506e892481b358a7cec50abdf320c71aa55904bfa

## "Build" (but it was pre-built)
FROM alpine:latest@sha256:e7c4abb69531cb09e2a2bbb56fad3367ab694865c49df898c1c683185cc4376c AS build

# TARGETARCH is set automatically by BuildKit (e.g., "amd64" or "arm64")
ARG TARGETARCH

WORKDIR /src

COPY temp/argo-diff-linux-${TARGETARCH} argo-diff

## ArgoCD
FROM ${ARGOCD_IMAGE} AS argocd
RUN argocd version --client

## Final image
FROM alpine:latest@sha256:e7c4abb69531cb09e2a2bbb56fad3367ab694865c49df898c1c683185cc4376c

# add new user
RUN adduser -D argo-diff

# install diff
RUN apk add --no-cache diffutils

WORKDIR /app

COPY --from=build --chown=argo-diff --chmod=755 /src/argo-diff argo-diff
COPY --from=argocd --chmod=755 /usr/local/bin/argocd /usr/local/bin/argocd

EXPOSE 8080

USER argo-diff
CMD ["./argo-diff"]
