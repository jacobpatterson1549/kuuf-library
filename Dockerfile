# download dependencies:
# make and bash to run the Makefile
# libwebp to encode images as webp
# download go dependencies for source code
FROM alpine:3.16 AS RUNNER
RUN apk add --no-cache \
        libwebp-tools=~1.2
WORKDIR /app

FROM golang:1.19-alpine3.16 AS BUILDER
WORKDIR /app
COPY go.mod go.sum ./
RUN apk add --no-cache \
        make=~4.3 \
        bash=~5.1 \
    && go mod download

# build the server
COPY . ./
RUN make build/kuuf-library \
        GO_ARGS="CGO_ENABLED=0"

# copy the server to a minimal build image
FROM RUNNER
COPY --from=BUILDER /app/build/kuuf-library ./
ENTRYPOINT [ "/app/kuuf-library" ]
