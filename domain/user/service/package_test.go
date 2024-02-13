// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
)

//go:generate go run go.uber.org/mock/mockgen -package service -destination state_mock_test.go github.com/juju/juju/domain/user/service State

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func mustNewUUID() user.UUID {
	uuid, err := user.NewUUID()
	if err != nil {
		panic(err)
	}
	return uuid
}
