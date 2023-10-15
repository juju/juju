// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package ec2 -destination context_mock_test.go github.com/juju/juju/environs/context ProviderCallContext

func Test(t *testing.T) {
	gc.TestingT(t)
}
