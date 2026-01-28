// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"
)

type importSuite struct{}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}
