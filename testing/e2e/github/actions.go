package github

import (
	"encoding/json"
	"fmt"

	"github.com/sethvargo/go-githubactions"
)

type Namer interface {
	Name() string
}

// NameMatrix returns a GitHub Actions matrix that will be used to dynamically
// generate a matrix of test jobs. This returns a JSON string that can be
// unmarshalled into a matrix https://github.blog/changelog/2020-04-15-github-actions-new-workflow-features/#new-fromjson-method-in-expressions.
func NameMatrix(namers []Namer) (string, error) {
	names := make([]string, len(namers))
	for i, n := range namers {
		names[i] = n.Name()
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

// SetOutput sets the GitHub Actions output with the given name and value.
// This panics if it can't write the output.
func SetOutput(name, value string) {
	githubactions.SetOutput(name, value)
}
