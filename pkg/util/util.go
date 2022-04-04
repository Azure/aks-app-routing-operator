// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package util

import (
	"context"
	"flag"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Upsert(ctx context.Context, c client.Client, res client.Object) error {
	// Use server-side apply to update resources and fall back to merge patch when
	// using fake clients in unit tests since they don't support SSA
	var patchType = client.Apply
	if flag.Lookup("test.v") != nil {
		patchType = client.Merge
	}

	err := c.Patch(ctx, res, patchType, client.FieldOwner("aks-app-routing-operator"), client.ForceOwnership)
	if errors.IsNotFound(err) {
		err = c.Create(ctx, res)
	}
	return err
}

func Int32Ptr(i int32) *int32 { return &i }
func Int64Ptr(i int64) *int64 { return &i }
func BoolPtr(b bool) *bool    { return &b }
