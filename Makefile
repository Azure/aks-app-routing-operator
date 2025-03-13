# Run `make help` for usage information on commands in this file.

.PHONY: help clean dev push e2e e2e-deploy unit crd manifests generate controller-gen

-include .env

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
CONTROLLER_TOOLS_VERSION ?= v0.17.2

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Environment variables for Terraform
SUB_ID=$(shell az account show -o json | jq -r '.id')
TENANT_ID=$(shell az account show -o json | jq -r '.tenantId')

help: ## Display this help.
	# prints all targets with comments next to them, extracted from this file
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

clean: ## Cleans the development environment state
	rm -rf devenv/state devenv/tf/.terraform.lock.hcl devenv/tf/.terraform devenv/tf/terraform.tfstate devenv/tf/terraform.tfstate.backup

# can have values of "public" or "private"
CLUSTER_TYPE="public"

dev: clean ## Deploys a development environment useful for testing the operator inside a cluster
	terraform --version
	cd devenv && mkdir -p state && cd tf && terraform init && TF_VAR_az_sub_id=$(SUB_ID) TF_VAR_az_tenant_id=$(TENANT_ID) terraform apply -auto-approve -var="clustertype=$(CLUSTER_TYPE)"
	./devenv/scripts/deploy_operator.sh

push: ## Pushes the current operator code to the current development environment
	echo "$(shell cat devenv/state/registry.txt)/app-routing-operator:$(shell date +%s)" > devenv/state/operator-image-tag.txt
	az acr login -n `cat devenv/state/registry.txt`
	docker build -t `cat devenv/state/operator-image-tag.txt` --file ./docker/operator.Dockerfile .
	docker push `cat devenv/state/operator-image-tag.txt`
	./devenv/scripts/push_image.sh

e2e: ## Runs end-to-end tests
	go run ./cmd/e2e/main.go infra --subscription=${SUBSCRIPTION_ID} --tenant=${TENANT_ID} --names=${INFRA_NAMES} 
	go run ./cmd/e2e/main.go deploy
	
e2e-deploy: ## runs only deploy
	go run ./cmd/e2e/main.go deploy

unit: ## Runs unit tests
	docker build ./devenv/ -t app-routing-dev:latest
	docker run --rm -v "$(shell pwd)":/usr/src/project -w /usr/src/project app-routing-dev:latest go test -race ./...

crd: generate manifests ## Generates all associated files from CRD

manifests: controller-gen ## Generate CRD manifest
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./api/..." output:crd:artifacts:config=config/crd/bases

generate: $(CONTROLLER_GEN) ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object paths="./api/..."


controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)