FROM golang:1.20 as builder

WORKDIR /go/src/github.com/Azure/aks-app-routing-operator
ADD . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -o e2e/e2e.test --count=1 --tags=e2e -c ./e2e

FROM scratch
WORKDIR /go/bin

COPY --from=builder /go/src/github.com/Azure/aks-app-routing-operator/e2e/e2e.test .