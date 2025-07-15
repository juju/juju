// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/juju/core/objectstore"
)

type mockObjectStore struct {
	objectstore.ObjectStore
}
