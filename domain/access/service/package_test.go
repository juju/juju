// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/user"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination state_mock_test.go github.com/juju/juju/domain/access/service State

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

func newUUID(c *tc.C) user.UUID {
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
