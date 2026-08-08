# temp/

Scratch directory. `temp/.gitignore` contains `*`, so **everything here is ignored** except the
`.gitignore` itself and this file. Nothing in it is ever committed, but two things expect the
directory to exist.

## Docker build staging

`Dockerfile` copies the prebuilt binary from here:

```dockerfile
COPY temp/argo-diff-linux-${TARGETARCH} argo-diff
```

GoReleaser writes those per-arch binaries during `dockerbuild.yml` / `release.yml` before buildx
runs. The image is *not* built from source in a builder stage — a local `docker build` fails unless
you populate `temp/argo-diff-linux-amd64` (or `-arm64`) yourself.

## Local webhook testing

`post-local.sh` (repo root) POSTs a captured GitHub webhook to a locally running server and reads:

- `temp/curl-comment-headers.txt` — the request headers (`--header @...`)
- `temp/curl-comment-payload.json` — the request body (`--data @...`)

Copy them out of the GitHub webhook delivery UI. Note the signature header only matters when the
server is *not* in dev mode; with `APP_ENV=dev` validation is skipped.

(`README.md` still names these `temp/curl-headers.txt` and `temp/curl-payload.json` — the script is
the authority.)

The `/dev` endpoint is usually easier for iteration: POST an `EventInfo` JSON document straight to
`http://127.0.0.1:8080/dev`, no headers or signature needed.
