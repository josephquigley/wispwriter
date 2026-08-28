# Build stage
#
# Compiles the binary and the front-end assets. Nothing from this stage
# reaches the final image except the four things copied out below.
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

# Resolve modules before copying the source, so editing a .go file does
# not invalidate the dependency layer and re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN make build VERSION="$WRITEFREELY_VERSION" && make ui

# Runtime stage
#
# One image serves both compose stacks. State (config.ini, keys, a SQLite
# database if used) lives in /data, which is the working directory, so the
# binary finds config.ini without a -c flag. The asset trees ship in the
# image under /usr/share/writefreely.
FROM alpine:3.22 AS runtime

LABEL org.opencontainers.image.source="https://github.com/writefreely/writefreely"
LABEL org.opencontainers.image.description="WriteFreely is a clean, minimalist publishing platform made for writers. Start a blog, share knowledge within your organization, or build a community around the shared act of writing."
LABEL org.opencontainers.image.licenses="AGPL-3.0"

RUN apk -U upgrade \
    && apk add --no-cache ca-certificates \
    && addgroup -g 1000 writefreely \
    && adduser -u 1000 -G writefreely -h /data -D writefreely

COPY --from=build /go/src/github.com/writefreely/writefreely/cmd/writefreely/writefreely /usr/bin/writefreely
COPY --from=build --chmod=755 /go/src/github.com/writefreely/writefreely/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY --from=build /go/src/github.com/writefreely/writefreely/pages /usr/share/writefreely/pages
COPY --from=build /go/src/github.com/writefreely/writefreely/static /usr/share/writefreely/static
COPY --from=build /go/src/github.com/writefreely/writefreely/templates /usr/share/writefreely/templates

# The asset trees stay root-owned and read-only to the app. Only the two
# directories the app writes to are handed over, and both are expected to
# be replaced by a bind mount at run time.
RUN mkdir -p /data /usr/share/writefreely/static/uploads \
    && chown writefreely:writefreely /data /usr/share/writefreely/static/uploads

# WRITEFREELY_DOCKER tells the interactive configurator it is in a
# container, so it binds 0.0.0.0 rather than localhost.
# WRITEFREELY_DOCKER_PARENT_DIR points it at this image's asset layout.
ENV WRITEFREELY_DOCKER=True
ENV WRITEFREELY_DOCKER_PARENT_DIR=/usr/share/writefreely
ENV WRITEFREELY_SERVICE_HINT=app
ENV HOME=/data

WORKDIR /data

EXPOSE 8080

# Defaults to uid 1000. Both compose files override this with PUID/PGID so
# a bind-mounted ./data does not have to be chowned to match.
USER writefreely

# The entrypoint generates keys when absent and applies pending
# migrations, then execs the binary.
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/bin/writefreely"]

# Accept any HTTP response as proof the server is up. Requiring a 2xx on
# "/" reports a healthy instance as unhealthy whenever the landing page
# isn't publicly readable: an unconfigured instance, or a single-user
# instance whose blog is password-protected.
HEALTHCHECK --start-period=5s --interval=15s --timeout=5s \
    CMD wget -q --spider --server-response http://localhost:8080/ 2>&1 | grep -q "HTTP/" || exit 1
