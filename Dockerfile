## Build
FROM golang:1.22 AS build

WORKDIR /src

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o argo-diff ./cmd/

## ArgoCD
FROM quay.io/argoproj/argocd:v2.12.6 AS argocd

## Final image
FROM alpine:latest

# add new user
RUN adduser -D argo-diff

# install diff
RUN apk add --no-cache diffutils

USER argo-diff
WORKDIR /app

COPY --from=build --chown=argo-diff /src/argo-diff argo-diff
COPY --from=build --chown=argo-diff /src/git-rev.txt git-rev.txt
COPY --from=argocd --chmod=755 /usr/local/bin/argocd /usr/local/bin/argocd

EXPOSE 8080

CMD ["./argo-diff"]
