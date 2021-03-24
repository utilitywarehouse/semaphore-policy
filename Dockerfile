FROM golang:1.16-alpine AS build
WORKDIR /go/src/github.com/utilitywarehouse/kube-policy-semaphore
COPY . /go/src/github.com/utilitywarehouse/kube-policy-semaphore
ENV CGO_ENABLED=0
RUN \
  apk --no-cache add git upx \
  && go get -t ./... \
  && go test -v \
  && go build -ldflags='-s -w' -o /kube-policy-semaphore . \
  && upx /kube-policy-semaphore

FROM alpine:3.13
COPY --from=build /kube-policy-semaphore /kube-policy-semaphore
ENTRYPOINT [ "/kube-policy-semaphore" ]
