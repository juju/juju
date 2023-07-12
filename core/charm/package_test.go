// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package charm -destination charm_mock_test.go github.com/juju/charm/v11 CharmMeta

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
