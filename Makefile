.PHONY: clean dev dev-private-cluster e2e e2e-private-cluster publish-tag push push-private

clean:
	rm -rf devenv/state devenv/.terraform.lock.hcl devenv/.terraform devenv/terraform.tfstate devenv/terraform.tfstate.backup

dev:
	terraform --version
	cd devenv && mkdir -p state && terraform init && terraform apply -auto-approve

dev-private-cluster:
	terraform --version
	cd devenv && mkdir -p state && terraform init && terraform apply -auto-approve -var="private-dns=true"
	cd devenv && /bin/sh scripts/deploy_addon_private_cluster.sh

e2e:
	go test -v --count=1 --tags=e2e ./e2e

e2e-private-cluster:
	# grab the image name and tag - that first gets determined by where and how we push it - so first need to build and push it - then export that name and registry and tag into the YAML
	az acr login -n `cat devenv/state/registry.txt`
	echo "$(shell cat devenv/state/registry.txt)/app-routing-operator-e2e:$(shell date +%s)" > devenv/state/e2e-image-tag.txt
	docker build --platform=linux/amd64 -t `cat devenv/state/e2e-image-tag.txt` -f e2e/Dockerfile .
	docker push `cat devenv/state/e2e-image-tag.txt`
	cd devenv && /bin/sh scripts/deploy_private_e2e_tester.sh


publish-tag:
	echo "$(shell cat devenv/state/registry.txt)/app-routing-operator:$(shell date +%s)" > devenv/state/operator-image-tag.txt
	az acr login -n `cat devenv/state/registry.txt`
	docker build -t `cat devenv/state/operator-image-tag.txt` .
	docker push `cat devenv/state/operator-image-tag.txt`

push: publish-tag
	kubectl set image -n kube-system --kubeconfig devenv/state/kubeconfig deployments/app-routing-operator operator=`cat devenv/state/operator-image-tag.txt`

push-private-cluster: publish-tag
	cd devenv && /bin/sh scripts/push_addon_image.sh