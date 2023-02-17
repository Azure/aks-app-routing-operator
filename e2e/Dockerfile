FROM golang:1.19 as builder

WORKDIR /go/src/github.com/Azure/aks-app-routing-operator
ADD ../. .
RUN go test -c -o e2e/e2e.test --count=1 --tags=e2e -c ./e2e

FROM golang:1.19
WORKDIR /go

COPY --from=builder /go/src/github.com/Azure/aks-app-routing-operator/e2e/e2e.test .