FROM mcr.microsoft.com/aks/devinfra/base-os-builder:master.250311.1-linux-amd64

ARG BUF_VERSION=1.7.0
ARG GRPC_GATEWAY_VERSION=2.11.2
ARG GRPC_GO_REDACT_VERSION=0.0.18
ARG GRPC_VERSION=1.64.0
ARG MOCKGEN_VERSION=0.4.0
ARG PROTOBUF_VERSION=1.34.1
ARG PROTOC_GEN_GO_GRPC_VERSION=1.3.0
ARG PROTOC_GO_INJECT_TAG_VERSION=1.4.0
ARG PROTOC_VERSION=27.0

RUN : && \
    go install "github.com/Azure/grpc-go-redact@v${GRPC_GO_REDACT_VERSION}" && \
    go install "github.com/favadi/protoc-go-inject-tag@v${PROTOC_GO_INJECT_TAG_VERSION}" && \
    go install "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v${GRPC_GATEWAY_VERSION}" && \
    go install "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v${GRPC_GATEWAY_VERSION}" && \
    go install "go.uber.org/mock/mockgen@v${MOCKGEN_VERSION}" && \
    go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@v${PROTOC_GEN_GO_GRPC_VERSION}" && \
    go install "google.golang.org/protobuf/cmd/protoc-gen-go@v${PROTOBUF_VERSION}" && \
    : && \
    curl -Lfo ./protoc.zip "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip" && \
    unzip -d /usr/local/ ./protoc.zip && \
    rm -f ./protoc.zip && \
    : && \
    curl -Lfo /usr/local/bin/buf "https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-$(uname -s)-$(uname -m)" && \
    chmod +x /usr/local/bin/buf && \
    : && \
    mkdir -m 777 /.cache/ && \
    chmod -R 777 /go/ && \
    ln -s /.netrc /root/ && \
    :
