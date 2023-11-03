## Build
FROM golang:1.21 AS build

WORKDIR /src

COPY argo-diff .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build

## Final image
FROM alpine:latest

COPY --from=build /src/argo-diff /app/argo-diff
COPY --from=build /src/git-rev.txt /app/git-rev.txt

WORKDIR /app

EXPOSE 8080

CMD ["./argo-diff"]
