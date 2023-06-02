// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package e2eutil

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"strings"
	"testing"
)

// Purge cleans up resources created by the previous run.
var Purge = func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	client, err := cfg.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create client during setup/purge: %s", err)
	}
	var list corev1.NamespaceList
	err = client.Resources().List(ctx, &list, func(opts *metav1.ListOptions) {
		opts.LabelSelector = "app.kubernetes.io/managed-by=e2eutil"
	})

	if err != nil {
		fmt.Printf("error while listing namespaces to purge past test runs: %s\n", err)
		return ctx, err
	}
	for _, item := range list.Items {
		fmt.Printf("cleaning up namespace from previous run %q\n", item.Name)
		err = client.Resources().Delete(ctx, &item)
		if err != nil {
			fmt.Printf("error while cleaning up namespace %q: %s\n", item.Name, err)
			return ctx, err
		}
	}

	return ctx, nil
}

func GetHostname(ns, domain string) string {
	return strings.ToLower(ns) + "." + domain
}

// CreateNSForTest creates a random namespace with the runID as a prefix. It is stored in the context
// so that the deleteNSForTest routine can look it up and delete it.
func CreateNSForTest(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
	prefix := "e2e-" + strings.ToLower(t.Name()) + "-"
	ns := envconf.RandomName(prefix, 10)
	ctx = context.WithValue(ctx, GetNamespaceKey(t), ns)

	t.Logf("Creating NS %s for test %s", ns, t.Name())
	nsObj := &corev1.Namespace{}
	nsObj.Name = ns
	return ctx, cfg.Client().Resources().Create(ctx, nsObj)
}

// DeleteNSForTest looks up the namespace corresponding to the given test and deletes it.
func DeleteNSForTest(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
	ns := fmt.Sprint(ctx.Value(GetNamespaceKey(t)))
	t.Logf("Deleting NS %v for test %v", ns, t.Name())

	nsObj := &corev1.Namespace{}
	nsObj.Name = ns
	return ctx, cfg.Client().Resources().Delete(ctx, nsObj)
}

// GetNamespaceKey returns the context key for a given test
func GetNamespaceKey(t *testing.T) string {
	// When we pass t.Name() from inside an `assess` step, the name is in the form TestName/Features/Assess
	if strings.Contains(t.Name(), "/") {
		return strings.Split(t.Name(), "/")[0]
	}

	// When pass t.Name() from inside a `testenv.BeforeEachTest` function, the name is just TestName
	return t.Name()
}
