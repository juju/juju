// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package charm -destination strategies_mock_test.go github.com/juju/juju/core/charm StateCharm,State,StoreCharm,Store,VersionValidator
//go:generate go run github.com/golang/mock/mockgen -package charm -destination lxdprofile_mock_test.go github.com/juju/juju/core/lxdprofile LXDProfile

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
