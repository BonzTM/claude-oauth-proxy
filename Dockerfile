ARG GO_VERSION=1.24.0

FROM golang:${GO_VERSION}-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT_SHORT=unknown
ARG BUILT_AT=unknown

RUN apk add --no-cache ca-certificates git

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY LICENSE README.md ./

RUN mkdir -p /rootfs/usr/local/bin /rootfs/home/nonroot/.config/claude-oauth-proxy /rootfs/var/lib/claude-oauth-proxy && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w -X github.com/bonztm/claude-oauth-proxy/internal/buildinfo.version=${VERSION} -X github.com/bonztm/claude-oauth-proxy/internal/buildinfo.commitShort=${COMMIT_SHORT} -X github.com/bonztm/claude-oauth-proxy/internal/buildinfo.builtAt=${BUILT_AT}" -o /rootfs/usr/local/bin/claude-oauth-proxy ./cmd/claude-oauth-proxy

FROM gcr.io/distroless/static-debian12:nonroot

ENV HOME=/home/nonroot \
    CLAUDE_OAUTH_PROXY_LISTEN_ADDR=:9999 \
    CLAUDE_OAUTH_PROXY_TOKEN_FILE=/var/lib/claude-oauth-proxy/tokens.json \
    CLAUDE_OAUTH_PROXY_NO_AUTO_LOGIN=true

WORKDIR /home/nonroot

COPY --from=builder --chown=nonroot:nonroot /rootfs/ /

EXPOSE 9999

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/claude-oauth-proxy"]
CMD ["serve"]
