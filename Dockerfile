## Build
FROM golang:1.25@sha256:6bac879c5b77e0fc9c556a5ed8920e89dab1709bd510a854903509c828f67f96 AS build

WORKDIR /src

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o argo-diff ./cmd/

## ArgoCD
FROM quay.io/argoproj/argocd:v2.14.15 AS argocd

## Final image
FROM alpine:latest

# add new user
RUN adduser -D argo-diff

# install diff
RUN apk add --no-cache diffutils

WORKDIR /app

COPY --from=build --chown=argo-diff --chmod=755 /src/argo-diff argo-diff
COPY --from=build --chown=argo-diff /src/git-rev.txt git-rev.txt
COPY --from=argocd --chmod=755 /usr/local/bin/argocd /usr/local/bin/argocd

EXPOSE 8080

USER argo-diff
CMD ["./argo-diff"]
