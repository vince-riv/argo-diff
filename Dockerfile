## Build
FROM golang:1.21 AS build

WORKDIR /src

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o argo-diff ./cmd/

## Final image
FROM alpine:latest

# add new user
RUN adduser -D argo-diff

USER argo-diff
WORKDIR /app

COPY --from=build --chown=argo-diff /src/argo-diff argo-diff
COPY --from=build --chown=argo-diff /src/git-rev.txt git-rev.txt

EXPOSE 8080

CMD ["./argo-diff"]
