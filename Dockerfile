## Build
FROM golang:1.25 AS build

WORKDIR /src

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o argo-diff ./cmd/

## ArgoCD
FROM quay.io/argoproj/argocd:v3.2.0@sha256:3c9b3b33a7e6c9c56a9d8417af1845afc516f714941962208b125017bbc1e4b1 AS argocd

## Final image
FROM alpine:latest@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412

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
