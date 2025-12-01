# download dependencies:
# - make to run the Makefile
# - libwebp-tools to encode webp images with /usr/bin/cwebp
# - sqlite, gcc, and musl-dev, and $CGO_ENABLED=1 for sqlite database support
FROM alpine:3.22 AS runner
WORKDIR /app
RUN apk add --no-cache \
        libwebp-tools=~1.5

# download go dependencies for source code
FROM golang:1.24-alpine3.22 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN apk add --no-cache \
        make=~4.4.1 \
        gcc=~14.2 \
        musl-dev=~1.2 \
    && go mod download

# build the server
COPY . ./
ARG CGO_ENABLED=0
ARG CGO_ENABLED=$CGO_ENABLED
RUN make build/kuuf-library \
        GO_ARGS="CGO_ENABLED=$CGO_ENABLED"

# copy the server to a minimal build image
FROM runner
COPY --from=builder /app/build/kuuf-library ./
ENTRYPOINT [ "/app/kuuf-library" ]
