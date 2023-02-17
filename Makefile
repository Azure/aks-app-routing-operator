.PHONY: clean-all clean-private clean-public dev-public-cluster update-image-on-deployment push-tester-image deploy-e2e run-e2e


# keep separate for simultaneous public/private dev without need for resource recreation
clean-public:
	rm -rf devenv/state devenv/public_cluster_tf/.terraform.lock.hcl devenv/public_cluster_tf/.terraform devenv/public_cluster_tf/terraform.tfstate devenv/public_cluster_tf/terraform.tfstate.backup

clean-private:
	rm -rf devenv/state devenv/private_cluster_tf/.terraform.lock.hcl devenv/private_cluster_tf/.terraform devenv/private_cluster_tf/terraform.tfstate devenv/private_cluster_tf/terraform.tfstate.backup

clean-all: clean-public clean-private

dev-public:
	terraform --version
	cd devenv && mkdir -p state && cd public_cluster_tf && terraform init && terraform apply -auto-approve

dev-private:
	terraform --version
	cd devenv && mkdir -p state && cd private_cluster_tf && terraform init && terraform apply -auto-approve
	cd devenv && /usr/bin/env sh scripts/deploy_addon_private_cluster.sh

push:
	echo "$(shell cat devenv/state/registry.txt)/app-routing-operator:$(shell date +%s)" > devenv/state/operator-image-tag.txt
	az acr login -n `cat devenv/state/registry.txt`
	docker build -t `cat devenv/state/operator-image-tag.txt` .
	docker push `cat devenv/state/operator-image-tag.txt`
	./devenv/scripts/push_image.sh

push-tester-image:
	# grab the image name and tag - that first gets determined by where and how we push it - so first need to build and push it - then export that name and registry and tag into the YAML
	az acr login -n `cat devenv/state/registry.txt`
	echo "$(shell cat devenv/state/registry.txt)/app-routing-operator-e2e:$(shell date +%s)" > devenv/state/e2e-image-tag.txt
	docker build -t `cat devenv/state/e2e-image-tag.txt` -f e2e/Dockerfile .
	docker push `cat devenv/state/e2e-image-tag.txt`

# deploy e2e job
e2e: push-tester-image
	./devenv/scripts/deploy_e2e_tester.sh

# to be run by e2e job inside the cluster
run-e2e:
	go test -v --count=1 --tags=e2e ./e2e