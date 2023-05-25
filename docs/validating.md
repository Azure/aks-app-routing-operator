
# Validating

Before submitting a pull request, you need to ensure that your changes work and don't cause regressions.

## Development Environment 

You can easily provision a development environment to test your changes on.

1. `az login` logs in the account you want to use
2. `az account set --subscription <subscription>` sets the subscription to use
4. `make clean-public` clears preexisitng development state
5. `make dev-public` provisions requires resources
6. `make push` builds and pushes the operator image to the cluster

You can test with a private cluster by replacing the `-public` suffix with `-private` in the commands.

Region can be specified by exporting an env variable before running the `make dev-public` command. `export TF_VAR_location="East US"` sets the location to East US.

This development environment is useful for manually interacting with App Routing and automated E2E testing.

## E2E

E2E is a set of automated tests ran against a development environment that validate the functionality of the operator. 

A prerequisite for running E2E is to have a [Development Environment](#development-environment) running. Once your development environment is ready, you can simply run E2E with `make e2e`.

