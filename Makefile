.PHONY: clean dev push push-tester-image e2e run-e2e

-include .env

# can have values of "public" or "private"
CLUSTER_TYPE="public"

clean:
	rm -rf devenv/state devenv/tf/.terraform.lock.hcl devenv/tf/.terraform devenv/tf/terraform.tfstate devenv/tf/terraform.tfstate.backup

dev:
	terraform --version
	cd devenv && mkdir -p state && cd tf && terraform init && terraform apply -auto-approve -var="clustertype=$(CLUSTER_TYPE)"
	./devenv/scripts/deploy_operator.sh

push:
	echo "$(shell cat devenv/state/registry.txt)/app-routing-operator:$(shell date +%s)" > devenv/state/operator-image-tag.txt
	az acr login -n `cat devenv/state/registry.txt`
	docker build -t `cat devenv/state/operator-image-tag.txt` .
	docker push `cat devenv/state/operator-image-tag.txt`
	./devenv/scripts/push_image.sh

e2e:
	# parenthesis preserve current working directory
	(cd testing/e2e && \
	 go run ./main.go infra --subscription=${SUBSCRIPTION_ID} --tenant=${TENANT_ID} --names=${INFRA_NAMES} --sp-app-obj-id=${SERVICE_PRINCIPAL_APP_OBJ_ID}&& \
	 go run ./main.go deploy)

unit:
	docker build ./devenv/ -t app-routing-dev:latest
	docker run --rm -v "$(shell pwd)":/usr/src/project -w /usr/src/project app-routing-dev:latest go test ./...

