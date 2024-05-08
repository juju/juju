// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package charm -destination charm_mock_test.go github.com/juju/charm/v13 CharmMeta
//go:generate go run go.uber.org/mock/mockgen -typed -package charm -destination core_charm_mock_test.go github.com/juju/juju/core/charm SelectorLogger,SelectorModelConfig

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
