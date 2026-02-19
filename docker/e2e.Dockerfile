FROM mcr.microsoft.com/oss/go/microsoft/golang:1.25.7 AS builder


WORKDIR /go/src/github.com/Azure/aks-app-routing-operator
ADD . .
RUN CGO_ENABLED=0 GOEXPERIMENT=nosystemcrypto GOOS=linux GOARCH=amd64 go build -v -a -ldflags '-extldflags "-static"' -o e2e cmd/e2e/main.go

FROM scratch
WORKDIR /
COPY --from=builder /go/src/github.com/Azure/aks-app-routing-operator/e2e .
ENTRYPOINT ["/e2e"]