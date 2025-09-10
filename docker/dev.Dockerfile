# convenience dockerfile for unit tests
# run make unit from root
FROM mcr.microsoft.com/oss/go/microsoft/golang:1.23

WORKDIR /app-routing-operator

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

RUN mkdir -p /usr/local/kubebuilder/bin
RUN wget -q https://github.com/etcd-io/etcd/releases/download/v3.5.0/etcd-v3.5.0-linux-amd64.tar.gz &&\
    tar xzf etcd-v3.5.0-linux-amd64.tar.gz &&\
    mv etcd-v3.5.0-linux-amd64/etcd /usr/local/kubebuilder/bin/
RUN wget -q https://storage.googleapis.com/kubernetes-release/release/v1.22.2/bin/linux/amd64/kube-apiserver &&\
    chmod +x kube-apiserver &&\
    mv kube-apiserver /usr/local/kubebuilder/bin/
