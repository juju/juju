// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package model_test -destination charm_mock_test.go github.com/juju/charm/v11 CharmMeta

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
