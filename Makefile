.PHONY: dev clean e2e

dev:
	terraform --version
	cd devenv && mkdir -p state && terraform init && terraform apply -auto-approve

clean:
	rm -rf devenv/state devenv/.terraform.lock.hcl devenv/.terraform devenv/terraform.tfstate devenv/terraform.tfstate.backup

e2e:
	go test -v --count=1 --tags=e2e ./e2e
