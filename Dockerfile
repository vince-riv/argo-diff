## Build
FROM golang:1.21 AS build

WORKDIR /src

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build

## Final image
FROM alpine:latest

COPY --from=build /src/argo-diff /app/argo-diff

WORKDIR /app

EXPOSE 8080

CMD ["./argo-diff"]
