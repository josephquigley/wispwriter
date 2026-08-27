# One image, two layouts.
#
#   docker build .                    the published image: everything under
#                                     /go, running as daemon
#   docker build --target fhs .       a filesystem-hierarchy layout: binary
#                                     in /usr/bin, assets in
#                                     /usr/share/writefreely, state in /data
#
# The default target is last, so a plain build produces what CI publishes.
# BuildKit only builds the stages a target needs, so asking for one layout
# does not build the other.

# ---------------------------------------------------------------- build ---
FROM golang:1.25-alpine3.22 AS build

RUN apk -U upgrade \
    && apk add --no-cache nodejs npm make g++ git \
    && npm install -g less less-plugin-clean-css

WORKDIR /go/src/github.com/writefreely/writefreely

# Enable the legacy OpenSSL provider before anything needs it. Appended
# rather than overwritten: replacing the file discards the system's own
# OpenSSL configuration.
COPY ossl_legacy.cnf .
RUN cat ossl_legacy.cnf >> /etc/ssl/openssl.cnf

ENV GO111MODULE=on
ENV NODE_OPTIONS=--openssl-legacy-provider

# Resolve modules before copying the source, so editing a .go file doesn't
# invalidate the dependency layer and re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN make build && make ui

# ----------------------------------------------------------- runtime base --
# Everything both layouts share. Labels and the healthcheck are inherited
# by the stages built from this one.
FROM alpine:3.22 AS runtime

LABEL org.opencontainers.image.source="https://github.com/writefreely/writefreely"
LABEL org.opencontainers.image.description="WriteFreely is a clean, minimalist publishing platform made for writers. Start a blog, share knowledge within your organization, or build a community around the shared act of writing."
LABEL org.opencontainers.image.licenses="AGPL-3.0"

RUN apk -U upgrade \
    && apk add --no-cache openssl ca-certificates

EXPOSE 8080

# Accept any HTTP response as proof the server is up. Requiring a 2xx on
# "/" reports a healthy instance as unhealthy whenever the landing page
# isn't publicly readable, such as an unconfigured instance or one whose
# blog is password-protected.
HEALTHCHECK --start-period=5s --interval=15s --timeout=5s \
    CMD wget -q --spider --server-response http://localhost:8080/ 2>&1 | grep -q "HTTP/" || exit 1

# -------------------------------------------------------------- fhs layout --
FROM runtime AS fhs

RUN mkdir -p /usr/share/writefreely/static/uploads /data \
    && addgroup -g 1000 writefreely \
    && adduser -u 1000 -G writefreely -h /data -D writefreely \
    && chown -R writefreely:writefreely /data /usr/share/writefreely/static/uploads

COPY --from=build /go/src/github.com/writefreely/writefreely/cmd/writefreely/writefreely /usr/bin/writefreely
COPY --from=build --chmod=755 /go/src/github.com/writefreely/writefreely/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY --from=build /go/src/github.com/writefreely/writefreely/pages /usr/share/writefreely/pages
COPY --from=build /go/src/github.com/writefreely/writefreely/static /usr/share/writefreely/static
COPY --from=build /go/src/github.com/writefreely/writefreely/templates /usr/share/writefreely/templates

ENV WRITEFREELY_DOCKER=True
ENV WRITEFREELY_DOCKER_PARENT_DIR=/usr/share/writefreely
ENV WRITEFREELY_SERVICE_HINT=app
ENV HOME=/data

WORKDIR /data

# /data holds config, keys and a SQLite database. Uploaded images live
# under the static asset tree, which is part of the image, so they need a
# volume of their own or an image upgrade destroys them.
VOLUME /data
VOLUME /usr/share/writefreely/static/uploads

# Runs as uid 1000. A bind-mounted ./data on the host must be owned by
# 1000:1000, or the container cannot write its config, keys or database.
USER writefreely

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/bin/writefreely"]

# ---------------------------------------------------------- default layout --
FROM runtime AS default

# Stage only what the runtime image needs: the binary, the entrypoint, the
# asset trees and the keys directory. The source tree is left behind.
COPY --from=build --chown=daemon:daemon \
    /go/src/github.com/writefreely/writefreely/cmd/writefreely/writefreely \
    /go/cmd/writefreely/writefreely
COPY --from=build --chown=daemon:daemon --chmod=755 \
    /go/src/github.com/writefreely/writefreely/docker-entrypoint.sh \
    /go/docker-entrypoint.sh
COPY --from=build --chown=daemon:daemon /go/src/github.com/writefreely/writefreely/templates /go/templates
COPY --from=build --chown=daemon:daemon /go/src/github.com/writefreely/writefreely/static /go/static
COPY --from=build --chown=daemon:daemon /go/src/github.com/writefreely/writefreely/pages /go/pages
COPY --from=build --chown=daemon:daemon /go/src/github.com/writefreely/writefreely/keys /go/keys

# static/uploads is not in the repository, so without creating it here the
# VOLUME below would be made by Docker as root, and an unprivileged
# process could not write to it.
RUN mkdir -p /go/static/uploads && chown daemon:daemon /go/static/uploads

WORKDIR /go

# Tell the interactive configurator it is running in a container, so it
# binds 0.0.0.0 rather than localhost, and point it at this layout.
ENV WRITEFREELY_DOCKER=True
ENV WRITEFREELY_DOCKER_PARENT_DIR=/go
ENV WRITEFREELY_SERVICE_HINT=writefreely-web

VOLUME /go/keys
VOLUME /go/static/uploads

USER daemon

# The entrypoint generates keys when absent, creates the schema on a first
# run and applies migrations, then execs the binary.
ENTRYPOINT ["/go/docker-entrypoint.sh"]
CMD ["cmd/writefreely/writefreely"]
