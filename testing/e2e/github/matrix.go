package github

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
)

// InfraMatrix returns a GitHub Actions matrix that will be used to dynamically
// generate a matrix of test jobs. This returns a JSON string that can be
// unmarshalled into a matrix https://github.blog/changelog/2020-04-15-github-actions-new-workflow-features/#new-fromjson-method-in-expressions.
func InfraMatrix(infras []infra.Provisioned) (string, error) {
	names := make([]string, len(infras))
	for i, infra := range infras {
		names[i] = infra.Name
	}

	matrix := matrix{
		"name": names,
	}
	b, err := json.Marshal(matrix)
	if err != nil {
		return "", fmt.Errorf("")
	}

	return string(b), nil
}

// matrix represents a GitHub Actions matrix that will be used to dynamically
// generate a matrix of test jobs.
// https://docs.github.com/en/actions/using-jobs/using-a-matrix-for-your-jobs
type matrix map[string][]string
