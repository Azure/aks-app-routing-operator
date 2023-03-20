#!/usr/bin/env bash
set -e

az acr login -n `cat devenv/state/registry.txt`
TAG=$(shell date +%s)
echo "$(shell cat devenv/state/registry.txt)/app-routing-operator-e2e:$TAG" > devenv/state/e2e-image-tag.txt
echo "$(shell cat devenv/state/registry.txt)/e2e-prom-client:$TAG" > devenv/state/e2e-prom-client.txt


docker build -t `cat devenv/state/e2e-prom-client.txt` -f e2e/fixtures/promclient/Dockerfile .
docker push `cat devenv/state/e2e-prom-client.txt`

docker build -t `cat devenv/state/e2e-image-tag.txt` -f e2e/Dockerfile .
docker push `cat devenv/state/e2e-image-tag.txt`
