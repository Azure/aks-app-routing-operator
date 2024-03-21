FROM golang:1.20 as builder

WORKDIR /go/src/github.com/Azure/aks-app-routing-operator
ADD . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -a -ldflags '-extldflags "-static"' -o aks-app-routing-operator

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /go/src/github.com/Azure/aks-app-routing-operator/aks-app-routing-operator .
COPY --from=builder /go/src/github.com/Azure/aks-app-routing-operator/config/crd/bases ./crd
ENTRYPOINT ["/aks-app-routing-operator"]
