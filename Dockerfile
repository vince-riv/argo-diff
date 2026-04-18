## Build
FROM golang:1.25@sha256:3760478c76cfe25533e06176e983e7808293895d48d15d0981c0cbb9623834e7 AS build

ARG VERSION=dev

WORKDIR /src

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X 'main.Version=${VERSION}'" -o argo-diff ./cmd/

## ArgoCD
FROM quay.io/argoproj/argocd:v3.3.6@sha256:16b92ba472fbb9287459cc52e0ecff07288dff461209955098edb56ce866fe49 AS argocd

## Final image
FROM alpine:latest@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

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
