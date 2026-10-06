ARG ARGOCD_IMAGE=quay.io/argoproj/argocd:v3.5.4@sha256:49dff79439bb38b1b942b19a113fe1fec7e6b6c671ddf9fe6a4a7c46bde0b77b

## "Build" (but it was pre-built)
FROM alpine:latest@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS build

# TARGETARCH is set automatically by BuildKit (e.g., "amd64" or "arm64")
ARG TARGETARCH

WORKDIR /src

COPY temp/argo-diff-linux-${TARGETARCH} argo-diff

## ArgoCD
FROM ${ARGOCD_IMAGE} AS argocd
RUN argocd version --client

## Final image
FROM alpine:latest@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

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
