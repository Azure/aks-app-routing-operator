// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package util

import (
	"context"
	"flag"
	"math/rand"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var patchType = client.Merge

func Upsert(ctx context.Context, c client.Client, res client.Object) error {
	// Use server-side apply to update resources and fall back to merge patch when
	// using fake clients in unit tests since they don't support SSA
	if flag.Lookup("test.v") == nil {
		patchType = client.Apply
	}

	err := c.Patch(ctx, res, patchType, client.FieldOwner("aks-app-routing-operator"), client.ForceOwnership)
	if errors.IsNotFound(err) {
		err = c.Create(ctx, res)
	}
	return err
}

// UseServerSideApply allows tests to require the server side apply patch strategy.
// Useful in cases where a real client that supports it is used.
// The default is to use Merge because SSA isn't supported by the fake client.
func UseServerSideApply() {
	patchType = client.Apply
}

func Int32Ptr(i int32) *int32      { return &i }
func Int64Ptr(i int64) *int64      { return &i }
func BoolPtr(b bool) *bool         { return &b }
func StringPtr(str string) *string { return &str }

func FindOwnerKind(owners []metav1.OwnerReference, kind string) string {
	for _, cur := range owners {
		if cur.Kind == kind {
			return cur.Name
		}
	}
	return ""
}

func Jitter(base time.Duration, ratio float64) time.Duration {
	if ratio >= 1 || ratio == 0 {
		return base
	}
	jitter := (rand.Float64() * float64(base) * ratio) - (float64(base) * (ratio / 2))
	return base + time.Duration(jitter)
}

func MergeMaps[M ~map[K]V, K comparable, V any](src ...M) M {
	merged := make(M)
	for _, m := range src {
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
}
