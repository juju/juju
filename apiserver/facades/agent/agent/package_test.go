// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package agent_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/agent CredentialService

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
