# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The AKS Web Application Routing Operator is a Kubernetes operator that manages resources for AKS Application Routing functionality. It's built using the Kubebuilder framework and manages several Custom Resource Definitions (CRDs) for routing, DNS, and certificate management.

## Development Commands

### Building and Testing
- `make help` - Display all available make targets
- `make unit` - Run unit tests using Docker development environment
- `make unit-vis` - Run unit tests and create visualization output
- `make docker-build-dev` - Build the development Docker image for testing

#### Running Unit Tests Locally (without Docker)

Some test packages (e.g. `pkg/controller/dns/`) use controller-runtime's envtest and require Kubernetes API server binaries. Use `setup-envtest` to install them and set `KUBEBUILDER_ASSETS`:

```bash
# Install setup-envtest if not already installed
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Install envtest binaries and run tests
setup-envtest use
export KUBEBUILDER_ASSETS="$(setup-envtest use -p path)"
go test ./pkg/controller/dns/ -v -count=1
```

If `setup-envtest use -p path` doesn't expand correctly in your shell, run `setup-envtest use` first to see the path, then set it directly:

```bash
KUBEBUILDER_ASSETS=<path from setup-envtest output> go test ./pkg/controller/dns/ -v -count=1
```

Packages that don't use envtest (e.g. `pkg/config/`, `pkg/controller/`, `pkg/manifests/`) can be tested directly with `go test`.

### Code Generation
- `make crd` - Generate all CRD-related files (calls generate and manifests)
- `make generate` - Generate DeepCopy, DeepCopyInto, and DeepCopyObject methods
- `make manifests` - Generate CRD manifests and RBAC configurations

### Development Environment
- `make clean` - Clean development environment state and Terraform files
- `make dev` - Deploy a complete development environment (AKS cluster + Azure resources)
- `make push` - Build and push operator image to development environment

### End-to-End Testing
- `make e2e` - Run complete end-to-end tests
- `make e2e-deploy` - Run only the deploy phase of e2e tests

## Architecture Overview

### Main Components

**Operator Entry Point**: `cmd/operator/main.go` - Main operator executable that sets up the manager and controllers

**Controller Manager**: `pkg/controller/controller.go` - Central manager that orchestrates all controllers:
- ExternalDNS controller for DNS management
- NginxIngressController reconciler for ingress management
- OSM (Open Service Mesh) integration controllers
- KeyVault secret provider class controllers
- Gateway API controllers (when enabled)
- Default domain certificate controllers (when enabled)

**Custom Resource Definitions**:
- `NginxIngressController` - Manages NGINX ingress controller deployments
- `ExternalDNS` (namespaced) - Manages DNS records for specific namespaces
- `ClusterExternalDNS` (cluster-scoped) - Manages DNS records cluster-wide
- `DefaultDomainCertificate` - Manages default TLS certificates

### Key Packages

**API Layer** (`api/v1alpha1/`): Contains CRD type definitions and validation logic

**Controllers** (`pkg/controller/`):
- `nginxingress/` - NGINX ingress controller management
- `dns/` - External DNS functionality
- `osm/` - Open Service Mesh integration
- `keyvault/` - Azure Key Vault secret management
- `service/` - Service-level ingress reconciliation
- `common/` - Shared controller utilities

**Configuration** (`pkg/config/`): Operator configuration management and validation

**Manifests** (`pkg/manifests/`): Kubernetes manifest generation utilities. 

**Testing Framework** (`testing/e2e/`): End-to-end testing infrastructure with support for multiple cluster configurations

### Controller Architecture

The operator uses controller-runtime with:
- Leader election for high availability (active-passive model)
- Field indexers for efficient resource lookups
- Custom caching strategies to reduce memory usage on large clusters
- Structured logging with zap logger
- Prometheus metrics integration

Key indexers:
- `nicIngressClassIndex` - Links ingress resources to NGINX ingress classes
- `gatewayListenerIndexName` - Associates Gateway listeners with service accounts

## Configuration

### Environment Variables and Flags
Configuration is handled through `pkg/config/config.go` with support for:
- Service account token path customization
- Metrics and health probe addresses
- Feature flags (Gateway API, Default Domain, etc.)
- Namespace and deployment name configuration

### Feature Flags
- `EnableGateway` - Enables Gateway API support
- `EnableDefaultDomain` - Enables default domain certificate management
- `DisableExpensiveCache` - Reduces memory usage by selective caching

## Testing Strategy

### Unit Tests
- Standard Go testing with Docker-based development environment
- Tests are located alongside source files (`*_test.go`)
- Mock implementations in `pkg/controller/testutils/`

### End-to-End Tests
- Comprehensive integration testing on real AKS clusters
- Multiple infrastructure configurations: basic, private, and OSM clusters
- Matrix testing across operator versions and configurations
- Automated testing on PRs via GitHub Actions

### Test Infrastructure
E2E tests support various cluster configurations defined in `testing/e2e/infra/infras.go`:
- `basic-cluster` - Standard public cluster
- `private-cluster` - Private AKS cluster
- `osm-cluster` - Cluster with Open Service Mesh enabled

## Development Workflow

### Local Development
1. Test changes with `make unit` before submitting PRs
2. If asked, verify changes with `make e2e`

### Development Environment Options
- `CLUSTER_TYPE` - Set to "public" or "private" for cluster type
- `TF_VAR_location` - Override default Azure region
- Infrastructure provisioning uses Terraform

### Pull Request Process
- E2E tests are triggered via `/ok-to-test sha=<sha>` comment by repository writers
- All unit tests must pass
- CRD generation must be up to date (`make crd`)

## Important Files and Directories

- `PROJECT` - Kubebuilder project configuration
- `config/crd/bases/` - Generated CRD manifests
- `devenv/` - Development environment Terraform and scripts
- `docker/` - Dockerfiles for operator and development images
- `docs/` - Additional documentation (e2e, local testing, releases)

## Azure Integration

The operator integrates deeply with Azure services:
- Azure DNS for external DNS management
- Azure Key Vault for certificate storage
- AKS-specific networking and ingress patterns
- Azure Container Registry for image management
- Azure RBAC and service principal authentication

## Common tasks

### Upgrade Go version

To upgrade the Go version perform the following steps
- Determine what the latest version of Go is 
- Upgrade the go.mod by changing the go directive to the new version of Go
- Run `go mod tidy`
- Upgrade the base dockerfiles to use the new Go image in ./docker directory
- Run `make unit` to ensure unit tests all pass
- Upgrade the go-version of actions/setup-go GitHub actions in .github/workflows directory