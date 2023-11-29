## Build
FROM golang:1.21 AS build

WORKDIR /src

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go test -v ./...
RUN CGO_ENABLED=0 GOOS=linux go build -o argo-diff ./cmd/

## Final image
FROM alpine:latest

COPY --from=build /src/argo-diff /app/argo-diff
COPY --from=build /src/git-rev.txt /app/git-rev.txt

WORKDIR /app

EXPOSE 8080

CMD ["./argo-diff"]
