FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26.4-alpine AS backend
RUN apk add --no-cache gcc musl-dev
WORKDIR /build/example
# Supply the standalone SDK with --build-context pluginapi=../ai-hub/common/pluginapi.
COPY --from=pluginapi / /build/ai-hub/common/pluginapi/
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=frontend /build/web/build ./web/build
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p dist && CGO_ENABLED=1 go build -trimpath \
    -ldflags='-s -w -linkmode external -extldflags "-static"' -o dist/example .

FROM backend AS package
RUN apk add --no-cache python3 && python3 scripts/package.py

FROM scratch AS release-export
COPY --from=package /build/example/dist/*.zip /

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 10001 example && adduser -D -u 10001 -G example example && \
    mkdir -p /data/plugin && chown 10001:10001 /data/plugin
COPY --from=backend /build/example/dist/example /usr/local/bin/example
COPY --chown=10001:10001 config.example.yaml /data/plugin/config.yaml
COPY --chmod=755 deploy/entrypoint.sh /usr/local/bin/example-entrypoint
WORKDIR /data/plugin
ENV EXAMPLE_SERVER_ADDRESS=0.0.0.0:3012
USER 10001:10001
EXPOSE 3012
ENTRYPOINT ["/usr/local/bin/example-entrypoint"]
CMD ["-config", "/data/plugin/config.yaml"]
