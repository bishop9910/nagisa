# Deliberately NO `# syntax=` directive.
#
# A syntax line makes BuildKit pull its own front-end image
# (`docker.io/docker/dockerfile:1.7`) *before* it parses anything else. That
# reference is hardcoded to Docker Hub and is resolved before any build arg
# exists, so REGISTRY_MIRROR below cannot redirect it — on a network that cannot
# reach registry-1.docker.io the build dies at step 1/3 with
# `failed to resolve image config for docker-image://docker.io/docker/dockerfile:1.7`
# long before the first FROM. Dropping the line uses BuildKit's built-in
# front end instead, and nothing here needs 1.7 features: the only modern syntax
# is `RUN --mount=type=cache`, which every BuildKit >= 1.2 understands. If you
# ever do need a specific front end, do not re-add this line for a mirrored
# network — pass the mirror explicitly instead:
#   docker buildx build --build-arg BUILDKIT_SYNTAX=${REGISTRY_MIRROR}docker/dockerfile:1.7 .
#
# Nagisa netdisk — one image that carries the service, the built front end and
# the generated API documentation.
#
# Three stages:
#   1. frontend  node + pnpm  -> web/dist        (Vite output, hashed assets)
#   2. backend   golang       -> /out/nagisa     (static, CGO_ENABLED=0)
#   3. runtime   alpine       -> /app            (~20 MB, non-root, uid 65532)
#
# REGISTRY_MIRROR is a build arg because every base image below comes from
# Docker Hub, which is unreachable on some networks. Set it to a mirror prefix
# (see deploy/compose/.env.example) or leave it empty for docker.io.
#
# Why this is not the old two-stage file:
#   * the front end is actually built here, so `/` is the netdisk UI instead of
#     the placeholder page the server serves when web/dist is absent;
#   * the Go module download is a cached layer of its own, so editing a .go file
#     no longer re-downloads the whole module graph;
#   * the binary is built with the version baked in (`-X main.Version`), because
#     `.git` is excluded from the build context and `make build` would otherwise
#     stamp an empty version;
#   * the service runs as uid 65532 rather than root.
#
# Why alpine and not distroless: distroless is the smaller, sharper runtime, but
# it lives on gcr.io and no Docker Hub mirror proxies gcr.io. On a network that
# cannot reach gcr.io the image is simply unbuildable, so the Docker Hub-based
# alpine is the base that actually works everywhere. The service is a static
# binary either way (CGO_ENABLED=0), so switching back is a two-line change.

ARG REGISTRY_MIRROR=

# ---------------------------------------------------------------------------
# 1. Front end
# ---------------------------------------------------------------------------
FROM ${REGISTRY_MIRROR}library/node:24-alpine AS frontend

# pnpm-lock.yaml is lockfileVersion 9.0; pin the major so a future pnpm 10
# cannot reinterpret it. Install through npm rather than corepack: corepack
# verifies package-manager signatures against keys baked into the Node release,
# which is a needless failure mode for a pinned, reproducible build.
ARG PNPM_VERSION=9
RUN npm install -g "pnpm@${PNPM_VERSION}" \
 && pnpm --version

WORKDIR /src/frontend

# Dependency manifests first: this layer is only rebuilt when the lockfile
# changes, not on every source edit. The store-dir flag (rather than a global
# pnpm config) keeps the content-addressable store inside the cache mount.
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --prod=false --store-dir=/pnpm/store

COPY frontend/ ./

# `pnpm run build` = vue-tsc --build + vite build. vite.config.ts writes the
# bundle to ../web/dist relative to frontend/, which is exactly ./web/dist here.
# VITE_DEV_API_TARGET only affects the dev server proxy, never a production
# bundle, so nothing environment specific is baked in. The two `test` guards
# turn a silent empty-output regression into a failed build.
RUN pnpm run build \
 && test -f /src/web/dist/index.html \
 && test -d /src/web/dist/assets

# ---------------------------------------------------------------------------
# 2. Back end
# ---------------------------------------------------------------------------
FROM ${REGISTRY_MIRROR}library/golang:1.25 AS backend

# CGO is off on purpose: modernc.org/sqlite is a pure-Go driver, so the result
# is a fully static binary with no libc to match against a runtime image. That
# is also what makes the runtime base swappable (alpine today, distroless or
# scratch if you can reach gcr.io).
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ENV CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOFLAGS=-trimpath

# The module proxy is a build arg for the same reason REGISTRY_MIRROR is: the
# upstream default (proxy.golang.org) is unreachable on exactly the networks
# that need a Docker Hub mirror, and a blocked `go mod download` looks like a
# broken repository rather than a network problem. `,direct` is the fallback to
# fetching from VCS for modules the proxy has not cached; module verification
# still runs against go.sum (a good mirror relays sumdb, so GOSUMDB needs no
# change). See deploy/compose/.env.example for a mirror that is known to work.
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /src

# go.mod/go.sum first, mirroring the frontend stage: the module cache is only
# invalidated by dependency changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    go mod download

COPY . .

# The generated protobuf stubs, ent client, wire_gen.go, configs/config.yaml and
# docs/ are all committed, so no code generation runs here — the image build
# needs no buf, no wire and no network beyond the module proxy.
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    go build -ldflags "-s -w -X main.Version=${VERSION}" -o /out/nagisa ./cmd/nagisa

# ---------------------------------------------------------------------------
# 3. Runtime
# ---------------------------------------------------------------------------
FROM ${REGISTRY_MIRROR}library/alpine:3.21 AS runtime

# ca-certificates for the S3 client over TLS, tzdata so log timestamps and any
# expiry maths follow the configured zone.
RUN apk add --no-cache ca-certificates tzdata

ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=unknown
LABEL org.opencontainers.image.title="nagisa-netdisk" \
      org.opencontainers.image.description="Kratos netdisk backend with an embedded web front end" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}"

WORKDIR /app

# Absolute paths, so nothing depends on the image working directory:
# configs/config.yaml resolves these as ${KEY:default} placeholders, and Kratos
# fills such a placeholder from the *config key* KEY — which its environment
# source derives only from the KRATOS_-prefixed variable. A bare
# `DATABASE_SOURCE=...` is a plain OS variable that nothing reads, and the
# placeholder would silently keep its (work-dir relative) default. The prefix is
# therefore load-bearing, not decoration. `KRATOS_<dotted.path>` is the other
# accepted form and is what the two addr overrides below use.
ENV KRATOS_DATABASE_SOURCE=/app/data/nagisa.db \
    KRATOS_PASSWORD_KEY_FILE=/app/data/password_key.pem \
    KRATOS_WEB_ROOT=/app/web/dist \
    KRATOS_server.http.addr=0.0.0.0:8000 \
    KRATOS_server.grpc.addr=0.0.0.0:9000

# The compiled service and the configuration it reads.
COPY --from=backend /out/nagisa /app/nagisa
COPY configs/ /app/configs/

# Static material the HTTP server publishes: the API docs under /docs/
# (internal/server/http.go serves ./docs) and the front end under WEB_ROOT.
COPY docs/ /app/docs/
COPY --from=frontend /src/web/dist/ /app/web/dist/

# /app/data holds the SQLite database and the RSA password key. Both it and the
# service are owned by uid/gid 65532 (the same numeric id distroless calls
# nonroot), because Docker copies a fresh named volume's ownership from the
# image — that is what lets the non-root container write without an init script.
#
# 0700 on the key file's directory is not needed: the key is created 0600 by the
# service itself (internal/pkg/crypt/rsa.go). /app/data stays 0755 so a bind
# mount with the same uid keeps working.
COPY --chown=65532:65532 deploy/compose/data/.keep /app/data/.keep

# Declared for `docker run -P` / tooling; Compose maps ports explicitly.
EXPOSE 8000 9000

# No VOLUME instruction: Compose declares the two volumes it actually needs, and
# an anonymous volume here would only hide the data on a plain `docker run`.
USER 65532:65532

# The service serves HTTP on 8000, gRPC on 9000 and the front end on /.
# Object storage and the JWT secret come from the environment; see
# deploy/compose/README.md for the full list.
ENTRYPOINT ["/app/nagisa"]
CMD ["-conf", "/app/configs"]
