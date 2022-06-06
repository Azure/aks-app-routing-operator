.PHONY: dev clean e2e push

dev:
	terraform --version
	cd devenv && mkdir -p state && terraform init && terraform apply -auto-approve

clean:
	rm -rf devenv/state devenv/.terraform.lock.hcl devenv/.terraform devenv/terraform.tfstate devenv/terraform.tfstate.backup

e2e:
	go test -v --count=1 --tags=e2e ./e2e

push:
	echo "$(shell cat devenv/state/registry.txt)/app-routing-operator:$(shell date +%s)" > devenv/state/operator-image-tag.txt
	az acr login -n `cat devenv/state/registry.txt`
	docker build -t `cat devenv/state/operator-image-tag.txt` .
	docker push `cat devenv/state/operator-image-tag.txt`
	kubectl set image --kubeconfig devenv/state/kubeconfig deployments/app-routing-operator operator=`cat devenv/state/operator-image-tag.txt`
