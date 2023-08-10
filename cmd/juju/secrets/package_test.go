// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsapi.go github.com/juju/juju/cmd/juju/secrets ListSecretsAPI,AddSecretsAPI,GrantRevokeSecretsAPI

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// NewAddCommandForTest returns a secrets command for testing.
func NewAddCommandForTest(store jujuclient.ClientStore, api AddSecretsAPI) *addSecretCommand {
	c := &addSecretCommand{
		secretsAPIFunc: func() (AddSecretsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewGrantCommandForTest returns a secrets command for testing.
func NewGrantCommandForTest(store jujuclient.ClientStore, api GrantRevokeSecretsAPI) *grantSecretCommand {
	c := &grantSecretCommand{
		secretsAPIFunc: func() (GrantRevokeSecretsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewRevokeCommandForTest returns a secrets command for testing.
func NewRevokeCommandForTest(store jujuclient.ClientStore, api GrantRevokeSecretsAPI) *revokeSecretCommand {
	c := &revokeSecretCommand{
		secretsAPIFunc: func() (GrantRevokeSecretsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewListCommandForTest returns a secrets command for testing.
func NewListCommandForTest(store jujuclient.ClientStore, listSecretsAPI ListSecretsAPI) *listSecretsCommand {
	c := &listSecretsCommand{
		listSecretsAPIFunc: func() (ListSecretsAPI, error) { return listSecretsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewShowCommandForTest returns a list-secrets command for testing.
func NewShowCommandForTest(store jujuclient.ClientStore, listSecretsAPI ListSecretsAPI) *showSecretsCommand {
	c := &showSecretsCommand{
		listSecretsAPIFunc: func() (ListSecretsAPI, error) { return listSecretsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}
