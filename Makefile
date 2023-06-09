.PHONY: clean dev push push-tester-image e2e run-e2e

# both can have values of "public" or "private"
CLUSTER_TYPE="public"
NUM_PRIVATE_ZONES="2"
NUM_PUBLIC_ZONES="2"


# keep separate for simultaneous public/private dev without need for resource recreation
clean:
	rm -rf devenv/state devenv/tf/.terraform.lock.hcl devenv/tf/.terraform devenv/tf/terraform.tfstate devenv/tf/terraform.tfstate.backup

dev:
	terraform --version
	cd devenv && mkdir -p state && cd tf && terraform init && terraform apply -auto-approve -var="clustertype=$(CLUSTER_TYPE)"
	#./devenv/scripts/deploy_operator.sh

push:
	echo "$(shell cat devenv/state/registry.txt)/app-routing-operator:$(shell date +%s)" > devenv/state/operator-image-tag.txt
	az acr login -n `cat devenv/state/registry.txt`
	docker build -t `cat devenv/state/operator-image-tag.txt` .
	docker push `cat devenv/state/operator-image-tag.txt`
	./devenv/scripts/push_image.sh

TAG = $(shell date +%s)
push-tester-images:
	az acr login -n `cat devenv/state/registry.txt`
	echo "$(shell cat devenv/state/registry.txt)/e2e-prom-client:$(TAG)" > devenv/state/e2e-prom-client.txt
	echo "$(shell cat devenv/state/registry.txt)/app-routing-operator-e2e:$(TAG)" > devenv/state/e2e-image-tag.txt
	docker build -t `cat devenv/state/e2e-prom-client.txt` -f e2e/fixtures/promclient/Dockerfile ./e2e/fixtures/promclient/
	docker push `cat devenv/state/e2e-prom-client.txt`
	docker build -t `cat devenv/state/e2e-image-tag.txt` -f e2e/Dockerfile .
	docker push `cat devenv/state/e2e-image-tag.txt`

# deploy e2e job
e2e: push-tester-images
	./devenv/scripts/deploy_e2e_tester.sh

# to be run by e2e job inside the cluster
run-e2e:
	go test -v --count=1 --tags=e2e ./e2e

# runs full test suite for all private cluster scenarios
private-cluster-test: clean
	./devenv/scripts/run_private_cluster.sh

# runs full test suite for all public cluster scenarios
public-cluster-test: clean
	./devenv/scripts/run_public_cluster.sh

all-tests:
	./devenv/scripts/run_private_cluster.sh
	make clean
	./devenv/scripts/run_public_cluster.sh

