# AKS Web Application Routing Operator

A Kubernetes operator that manages resources related to AKS Application Routing functionality.

## Docs

View the [docs](docs/) folder for more information.

## Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.opensource.microsoft.com.

When you submit a pull request, a CLA bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

## E2E
This project leverages Terraform and the local user's Azure credentials to run an extensive E2E suite.

To run a single E2E suite, the user must run the following commands from the root of the project:
1. `make clean` to clear out any preexisting Terraform state.
2. `make dev` to deploy all Azure resources necessary to run a full suite, including a Kubernetes cluster that the operator will be deployed to. IMPORTANT: this does not start the add-on. The next step needs to be run for the add-on to be fully deployed with the correct image. 
3. `make push` to build the add-on image according to the user's local state/branch and push it to the add-on deployment. This step can be re-run when changes to the local add-on are made and the user wishes to manually test those changes on a cluster.
4. `make e2e` to deploy the e2e tester job, which will run the e2e test suite inside the cluster.

By default, the `make dev` command will create an environment with a public cluster using a public DNS Zone. However, two arguments can be specified to change the type of the cluster and/or the zone: `CLUSTER_TYPE` and `DNS_ZONE_TYPE`. For instance, to run a suite with a private cluster and a public zone, a user can run `	make dev CLUSTER_TYPE=private DNS_ZONE_TYPE=public`.

Alternatively, to run a full suite with all possible configuration options, users can simply run `make all`.

## Trademarks

This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft 
trademarks or logos is subject to and must follow 
[Microsoft's Trademark & Brand Guidelines](https://www.microsoft.com/en-us/legal/intellectualproperty/trademarks/usage/general).
Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship.
Any use of third-party trademarks or logos are subject to those third-party's policies.
