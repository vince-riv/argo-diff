ARG ARGOCD_IMAGE=quay.io/argoproj/argocd:v3.5.6@sha256:8e5cc5bb008df8a09dba439462e58c94c55b7735888d3ebde0c25fc05e4e5335

## "Build" (but it was pre-built)
FROM alpine:latest@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS build

# TARGETARCH is set automatically by BuildKit (e.g., "amd64" or "arm64")
ARG TARGETARCH

WORKDIR /src

COPY temp/argo-diff-linux-${TARGETARCH} argo-diff

## ArgoCD
FROM ${ARGOCD_IMAGE} AS argocd
RUN argocd version --client

## Final image
FROM alpine:latest@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

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
