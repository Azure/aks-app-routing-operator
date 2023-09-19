
# Local testing 

Before submitting a pull request, you need to ensure that your changes work and don't cause regressions. You may also need to inspect what's going on inside the cluster to develop features.

## Development Environment 

You can easily provision a development environment to test your changes on.

1. `make clean` to clear out any preexisting Terraform state.
2. `make dev` to deploy all Azure resources necessary to run a full suite, including a Kubernetes cluster that the operator will be deployed to. IMPORTANT: this does not start the add-on. The next step needs to be run for the add-on to be fully deployed with the correct image.
3. `make push` to build the add-on image according to the user's local state/branch and push it to the add-on deployment. This step can be re-run when changes to the local add-on are made and the user wishes to manually test those changes on a cluster.

By default, the `make dev` command will create an environment with a public cluster using a public DNS Zone. However, two arguments can be specified to change the type of the cluster and/or the zone: `CLUSTER_TYPE` and `DNS_ZONE_TYPE`. For instance, to run a suite with a private cluster and a public zone, a user can run `	make dev CLUSTER_TYPE=private DNS_ZONE_TYPE=public`.

Region can be specified by exporting an env variable before running the `make dev` command. `export TF_VAR_location="East US"` sets the location to East US.

This development environment is useful for manually interacting with App Routing during development.

## E2E

E2E is a set of automated tests that prevent regressions and ensure new features. Any new features should be thoroughly tested with E2E. See [E2E](./e2e.md) for information.
