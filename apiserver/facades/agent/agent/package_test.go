// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package agent -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/agent CredentialService,AgentPasswordService

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
