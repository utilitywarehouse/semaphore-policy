FROM golang:1.24-alpine AS build
WORKDIR /go/src/github.com/utilitywarehouse/semaphore-policy
COPY . /go/src/github.com/utilitywarehouse/semaphore-policy
ENV CGO_ENABLED=0
RUN \
  apk --no-cache add git \
    && go get -t ./... \
    && go test -v \
    && go build -ldflags='-s -w' -o /semaphore-policy .

FROM alpine:3.22
COPY --from=build /semaphore-policy /semaphore-policy
ENTRYPOINT [ "/semaphore-policy" ]
