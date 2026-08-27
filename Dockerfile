# Build image
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

# Stage only what the runtime image needs: the binary, the asset trees and
# the keys directory. The source tree is deliberately left behind.
RUN make build \
    && make ui \
    && mkdir -p /stage/cmd/writefreely \
    && cp cmd/writefreely/writefreely /stage/cmd/writefreely/writefreely \
    && cp docker-entrypoint.sh /stage/docker-entrypoint.sh \
    && chmod +x /stage/docker-entrypoint.sh \
    && cp -R templates static pages keys /stage

# Final image
FROM alpine:3.22

LABEL org.opencontainers.image.source="https://github.com/writefreely/writefreely"
LABEL org.opencontainers.image.description="WriteFreely is a clean, minimalist publishing platform made for writers. Start a blog, share knowledge within your organization, or build a community around the shared act of writing."
LABEL org.opencontainers.image.licenses="AGPL-3.0"

RUN apk -U upgrade \
    && apk add --no-cache openssl ca-certificates

COPY --from=build --chown=daemon:daemon /stage /go

WORKDIR /go

# Tell the interactive configurator it is running in a container, so it
# binds 0.0.0.0 rather than localhost, and point it at this image's layout.
ENV WRITEFREELY_DOCKER=True
ENV WRITEFREELY_DOCKER_PARENT_DIR=/go
ENV WRITEFREELY_SERVICE_HINT=writefreely-web

VOLUME /go/keys

# Uploaded images are written under the static asset tree, which is part
# of the image. Without a volume, every upload is destroyed on the next
# container recreate -- which is what an image upgrade does.
VOLUME /go/static/uploads

EXPOSE 8080
USER daemon

# The entrypoint generates keys when absent and applies pending
# migrations, then execs the binary. Overriding CMD still works:
# `docker run <image> cmd/writefreely/writefreely --config` reaches the
# binary with those arguments.
ENTRYPOINT ["/go/docker-entrypoint.sh"]
CMD ["cmd/writefreely/writefreely"]

# Accept any HTTP response as proof the server is up. Testing for a 2xx on
# "/" reports a healthy instance as unhealthy whenever the landing page
# isn't publicly readable -- an unconfigured instance, or a single-user
# instance whose blog is password-protected.
HEALTHCHECK --start-period=5s --interval=15s --timeout=5s \
    CMD wget -q --spider --server-response http://localhost:8080/ 2>&1 | grep -q "HTTP/" || exit 1
