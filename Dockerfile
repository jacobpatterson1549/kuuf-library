# download dependencies:
# make and bash to run the Makefile
# libwebp to encode images as webp
# download go dependencies for source code
FROM golang:1.19-alpine3.16 AS BUILDER
WORKDIR /app
COPY go.mod go.sum ./
RUN apk add --no-cache \
        build-base=~0.5 \
        make=~4.3 \
        bash=~5.1 \
    && go mod download

# build the server
COPY . ./
RUN make build/kuuf-library

# copy the server to a minimal build image
FROM scratch
WORKDIR /app
COPY --from=BUILDER /app/build/kuuf-library ./
ENTRYPOINT [ "/app/kuuf-library" ]
