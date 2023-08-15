FROM golang:1.20 as builder

WORKDIR /go/src/e2e
ADD . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -a -ldflags '-extldflags "-static"' -o e2e

FROM scratch
WORKDIR /
COPY --from=builder /go/src/e2e/e2e .
ENTRYPOINT ["/e2e"]