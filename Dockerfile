## Build
FROM golang:1.25 AS build

ARG VERSION=dev

WORKDIR /src

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X 'main.Version=${VERSION}'" -o argo-diff ./cmd/

## ArgoCD
FROM quay.io/argoproj/argocd:v3.2.1@sha256:a8532a23ed5f6e65afaf2a082b65fc74614549e54774f6b25fe3c993faa7bea3 AS argocd

## Final image
FROM alpine:latest@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62

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
