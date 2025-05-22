// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

//go:generate go run go.uber.org/mock/mockgen -typed -package agent -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/agent CredentialService,AgentPasswordService
