// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package charm -destination strategies_mock_test.go github.com/juju/juju/core/charm StateCharm,State,StoreCharm,Store,JujuVersionValidator
//go:generate go run go.uber.org/mock/mockgen -package charm -destination lxdprofile_mock_test.go github.com/juju/juju/core/lxdprofile LXDProfile
//go:generate go run go.uber.org/mock/mockgen -package charm -destination charm_mock_test.go github.com/juju/charm/v8 CharmMeta

func Test(t *testing.T) {
	gc.TestingT(t)
}
