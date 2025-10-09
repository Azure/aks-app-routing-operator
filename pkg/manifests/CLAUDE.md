# Manifests

This package stores Kubernetes manifests that are served to customers. These manifests are used to deploy various components of the App Routing Operator.

# Regenerating Test Fixtures

The test fixtures in this package are generated from the Helm charts. To regenerate the fixtures, run the tests with the `GENERATE_FIXTURES` environment variable set to `true`:

```bash
GENERATE_FIXTURES=true go test -v ./pkg/manifests
```
