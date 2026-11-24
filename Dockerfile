ARG ARGOCD_IMAGE=quay.io/argoproj/argocd:v3.6.3@sha256:f2e45010dbd6aeff01b845d212b436b2d8e7f44d90d437274906c2ab21e59e0d

## "Build" (but it was pre-built)
FROM alpine:latest@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS build

# TARGETARCH is set automatically by BuildKit (e.g., "amd64" or "arm64")
ARG TARGETARCH

WORKDIR /src

COPY temp/argo-diff-linux-${TARGETARCH} argo-diff

## ArgoCD
FROM ${ARGOCD_IMAGE} AS argocd
RUN argocd version --client

## Final image
FROM alpine:latest@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

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
