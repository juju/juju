// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
)

// IsBlobStored returns true if a given storage path is in used in the
// managed blob store.
func IsBlobStored(c *gc.C, store objectstore.ObjectStore, storagePath string) bool {
	r, _, err := store.Get(context.Background(), storagePath)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return false
		}
		c.Fatalf("Get failed: %v", err)
		return false
	}
	r.Close()
	return true
}
