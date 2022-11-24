# download dependencies:
# - make to run the Makefile
# - libwebp-tools to encode webp images with /usr/bin/cwebp
FROM alpine:3.16 AS RUNNER
WORKDIR /app
RUN apk add --no-cache \
        libwebp-tools=~1.2

# download go dependencies for source code
FROM golang:1.19-alpine3.16 AS BUILDER
WORKDIR /app
COPY go.mod go.sum ./
RUN apk add --no-cache \
        make=~4.3 \
    && go mod download

# build the server
COPY . ./
RUN make build/kuuf-library \
        GO_ARGS="CGO_ENABLED=0"

# copy the server to a minimal build image
FROM RUNNER
COPY --from=BUILDER /app/build/kuuf-library ./
ENTRYPOINT [ "/app/kuuf-library" ]
