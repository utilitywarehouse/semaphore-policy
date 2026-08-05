FROM golang:1.26-alpine AS build
WORKDIR /go/src/github.com/utilitywarehouse/semaphore-policy
COPY . /go/src/github.com/utilitywarehouse/semaphore-policy
ENV CGO_ENABLED=0
# GOTOOLCHAIN pins the exact toolchain declared by go.mod's `go` line, so the
# build isn't at the mercy of whatever patch version the base image ships.
RUN \
  apk --no-cache add git \
    && GOTOOLCHAIN=go$(awk '/^go /{print $2; exit}' go.mod) \
    && go mod download \
    && go test -v \
    && go build -ldflags='-s -w' -o /semaphore-policy .

FROM alpine:3.24
COPY --from=build /semaphore-policy /semaphore-policy
ENTRYPOINT [ "/semaphore-policy" ]
