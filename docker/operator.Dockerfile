FROM mcr.microsoft.com/oss/go/microsoft/golang:1.25.7 AS builder

WORKDIR /go/src/github.com/Azure/aks-app-routing-operator
ADD . .
# nosystemcrypto: Microsoft's Go fork enables systemcrypto (OpenSSL via CGo) by default.
# We need CGO_ENABLED=0 for a static binary that runs on distroless, so opt out.
RUN CGO_ENABLED=0 GOEXPERIMENT=nosystemcrypto GOOS=linux GOARCH=amd64 go build -v -a -ldflags '-extldflags "-static"' -o aks-app-routing-operator cmd/operator/main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /go/src/github.com/Azure/aks-app-routing-operator/aks-app-routing-operator .
COPY --from=builder /go/src/github.com/Azure/aks-app-routing-operator/config/crd/bases ./crd
ENTRYPOINT ["/aks-app-routing-operator"]
