# download dependencies:
# - make to run the Makefile
# - libwebp-tools to encode webp images with /usr/bin/cwebp
# - sqlite, gcc, and musl-dev, and $CGO_ENABLED=1 for sqlite database support
FROM alpine:3.16 AS runner
WORKDIR /app
RUN apk add --no-cache \
        libwebp-tools=~1.2

# download go dependencies for source code
FROM golang:1.19-alpine3.16 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN apk add --no-cache \
        make=~4.3 \
        gcc=~11.2 \
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
