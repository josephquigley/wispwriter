# The binary goes where binaries go, the assets where read-only shared
# data goes, and everything the instance writes into a single state
# directory. One volume to back up, and nothing mutable inside the tree
# that ships with the image.
#
# The state directory is /data, which is where upstream's Dockerfile.prod
# puts it. A more conventional choice would be /var/lib/writefreely, but
# matching upstream means an operator moving between the two images keeps
# the same bind mount.
#
#   /usr/bin/writefreely                             binary
#   /usr/share/writefreely/{templates,static,pages}  read-only assets
#   /data/                                           config, keys, database, uploads

# --------------------------------------------------------------- assets ---
# The CSS and the prose bundle are architecture independent, so they are
# built once on the builder's own platform rather than once per target
# under emulation. Node crashes with an illegal instruction often enough
# under qemu-user on linux/arm64 to fail the build outright, and even when
# it survives, running npm install and webpack once per architecture costs
# minutes for identical output.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine3.22 AS assets

RUN apk -U upgrade \
    && apk add --no-cache nodejs npm make g++ git \
    && npm install -g less less-plugin-clean-css

# webpack 4 hashes with md4, which OpenSSL 3 refuses by default. Node's
# own flag is what re-enables it: appending a legacy provider section to
# /etc/ssl/openssl.cnf, as this file used to do, has no effect on Node.
ENV NODE_OPTIONS=--openssl-legacy-provider

WORKDIR /src
COPY . .

# less writes into static/css and webpack into static/js.
RUN make ui

# ---------------------------------------------------------------- build ---
FROM golang:1.25-alpine3.22 AS build

# gcc and musl-dev are for cgo, which the sqlite build tag needs. Node is
# not installed here: the assets stage above builds everything that needs
# it.
RUN apk -U upgrade \
    && apk add --no-cache make gcc musl-dev git

WORKDIR /go/src/github.com/writefreely/writefreely

# git describe cannot work in every build context (notably a git
# worktree, whose .git is a file pointing outside the context), and CI
# checkouts are shallow and carry no tags. Pass the version in so the
# binary never reports a bare "v".
ARG WRITEFREELY_VERSION=

# Resolve modules before copying the source, so editing a .go file doesn't
# invalidate the dependency layer and re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Overwrites the checked-in static assets with the ones just built.
COPY --from=assets /src/static ./static

RUN make build VERSION="$WRITEFREELY_VERSION"

# ------------------------------------------------------------- runtime ---
FROM alpine:3.22

LABEL org.opencontainers.image.source="https://github.com/josephquigley/writefreely-wisp"
LABEL org.opencontainers.image.title="WriteFreely (Wisp Edition)"
LABEL org.opencontainers.image.description="WriteFreely (Wisp Edition) is a fork of WriteFreely, a clean, minimalist publishing platform made for writers. This edition adds post management, image uploads, multiple verification links, reorderable pinned posts, and subscribe button options."
LABEL org.opencontainers.image.licenses="AGPL-3.0"

RUN apk -U upgrade \
    && apk add --no-cache ca-certificates \
    && mkdir -p /usr/share/writefreely /data/uploads \
    && addgroup -g 1000 writefreely \
    && adduser -u 1000 -G writefreely -h /data -D writefreely \
    && chown -R writefreely:writefreely /data

COPY --from=build /go/src/github.com/writefreely/writefreely/cmd/writefreely/writefreely /usr/bin/writefreely
COPY --from=build --chmod=755 /go/src/github.com/writefreely/writefreely/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
# Carried in the image so an operator switching an upstream install over
# can run it with docker run --entrypoint, without cloning the repository.
COPY --from=build --chmod=755 /go/src/github.com/writefreely/writefreely/scripts/switch-from-writefreely.sh /usr/local/bin/switch-from-writefreely
COPY --from=build /go/src/github.com/writefreely/writefreely/pages /usr/share/writefreely/pages
COPY --from=build /go/src/github.com/writefreely/writefreely/static /usr/share/writefreely/static
COPY --from=build /go/src/github.com/writefreely/writefreely/templates /usr/share/writefreely/templates

# Tells the interactive configurator it is in a container, so it binds
# 0.0.0.0 rather than localhost, and points it at the asset root.
ENV WRITEFREELY_DOCKER=True
ENV WRITEFREELY_DOCKER_PARENT_DIR=/usr/share/writefreely
ENV WRITEFREELY_SERVICE_HINT=app

# The working directory is the state directory, so the binary's default
# config.ini lookup finds the mounted one without a -c flag.
WORKDIR /data

EXPOSE 8080

# Defaults to uid 1000. Both compose files override this with PUID and
# PGID, so a bind-mounted state directory does not have to be chowned to
# match a uid baked into the image.
USER writefreely

# The entrypoint generates keys when absent, creates the schema on a first
# run and applies migrations, then execs the binary.
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/bin/writefreely"]

# Accept any HTTP response as proof the server is up. Requiring a 2xx on
# "/" reports a healthy instance as unhealthy whenever the landing page
# isn't publicly readable, such as an unconfigured instance or one whose
# blog is password-protected.
HEALTHCHECK --start-period=5s --interval=15s --timeout=5s \
    CMD wget -q --spider --server-response http://localhost:8080/ 2>&1 | grep -q "HTTP/" || exit 1
