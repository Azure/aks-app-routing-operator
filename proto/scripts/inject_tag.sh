#!/usr/bin/env sh

find "${PROTOS_FOLDER-.}" -iname "*.pb.go" | \
    xargs -P "$(getconf _NPROCESSORS_ONLN)" -I{} \
    bash -c "echo 'protoc-go-inject-tag {}' && protoc-go-inject-tag -input={} -verbose"
