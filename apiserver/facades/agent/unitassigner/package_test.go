// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package unitassigner -destination package_mock_test.go github.com/juju/juju/apiserver/facades/agent/unitassigner SpaceService
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
