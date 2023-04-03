FROM golang:1.20 as builder

WORKDIR /go/src/prom-client
ADD . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -a -ldflags '-extldflags "-static"' -o prom-client

FROM scratch
WORKDIR /
COPY --from=builder /go/src/prom-client/prom-client .
ENTRYPOINT ["/prom-client"]
