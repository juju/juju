// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package model_test -destination charm_mock_test.go github.com/juju/juju/internal/charm CharmMeta

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
