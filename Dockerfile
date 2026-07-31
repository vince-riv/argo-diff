## "Build" (but it was pre-built)
FROM alpine:latest@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS build

# TARGETARCH is set automatically by BuildKit (e.g., "amd64" or "arm64")
ARG TARGETARCH

WORKDIR /src

COPY temp/argo-diff-linux-${TARGETARCH} argo-diff

## ArgoCD
FROM quay.io/argoproj/argocd:v3.4.6@sha256:6e9f4f1d646d9056c8e285495d0c8043b5f553c784181b3522ef324dcefdcc82 AS argocd

## Final image
FROM alpine:latest@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

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
