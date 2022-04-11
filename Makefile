dev:
	cd devenv && mkdir -p state && terraform init && terraform apply -auto-approve

clean:
	rm -rf devenv/state devenv/.terraform.lock.hcl devenv/.terraform devenv/terraform.tfstate devenv/terraform.tfstate.backup

