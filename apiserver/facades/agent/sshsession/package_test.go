// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package sshsession_test -destination context_mocks_test.go github.com/juju/juju/apiserver/facade Authorizer,Context,Resources
//go:generate go run go.uber.org/mock/mockgen -package sshsession_test -destination mocks_test.go github.com/juju/juju/apiserver/facades/agent/sshsession Backend

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
