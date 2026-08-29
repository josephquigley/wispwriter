# The binary goes where binaries go, the assets where read-only shared
# data goes, and everything the instance writes into a single state
# directory. One volume to back up, and nothing mutable inside the tree
# that ships with the image.
#
#   /usr/bin/writefreely                             binary
#   /usr/share/writefreely/{templates,static,pages}  read-only assets
#   /var/lib/writefreely/                            config, keys, database, uploads

# ---------------------------------------------------------------- build ---
FROM golang:1.25-alpine3.22 AS build

RUN apk -U upgrade \
    && apk add --no-cache nodejs npm make g++ git \
    && npm install -g less less-plugin-clean-css

WORKDIR /go/src/github.com/writefreely/writefreely

# webpack 4 hashes with md4, which OpenSSL 3 refuses by default. Node's
# own flag is what re-enables it: appending a legacy provider section to
# /etc/ssl/openssl.cnf, as this file used to do, has no effect on Node.
ENV NODE_OPTIONS=--openssl-legacy-provider

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

RUN make build VERSION="$WRITEFREELY_VERSION" && make ui

# ------------------------------------------------------------- runtime ---
FROM alpine:3.22

LABEL org.opencontainers.image.source="https://github.com/josephquigley/wispwriter"
LABEL org.opencontainers.image.title="WriteFreely (Wisp Edition)"
LABEL org.opencontainers.image.description="WriteFreely (Wisp Edition) is a fork of WriteFreely, a clean, minimalist publishing platform made for writers. This edition adds post management, image uploads, multiple verification links, reorderable pinned posts, and subscribe button options."
LABEL org.opencontainers.image.licenses="AGPL-3.0"

RUN apk -U upgrade \
    && apk add --no-cache ca-certificates \
    && mkdir -p /usr/share/writefreely /var/lib/writefreely/uploads \
    && addgroup -g 1000 writefreely \
    && adduser -u 1000 -G writefreely -h /var/lib/writefreely -D writefreely \
    && chown -R writefreely:writefreely /var/lib/writefreely

COPY --from=build /go/src/github.com/writefreely/writefreely/cmd/writefreely/writefreely /usr/bin/writefreely
COPY --from=build --chmod=755 /go/src/github.com/writefreely/writefreely/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
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
WORKDIR /var/lib/writefreely

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
