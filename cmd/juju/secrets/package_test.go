// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/juju/jujuclient"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/secretsapi.go github.com/juju/juju/cmd/juju/secrets ListSecretsAPI,AddSecretsAPI,GrantRevokeSecretsAPI,UpdateSecretsAPI,RemoveSecretsAPI

// NewAddCommandForTest returns a secrets command for testing.
func NewAddCommandForTest(store jujuclient.ClientStore, api AddSecretsAPI) *addSecretCommand {
	c := &addSecretCommand{
		secretsAPIFunc: func(ctx context.Context) (AddSecretsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewUpdateCommandForTest returns a secrets command for testing.
func NewUpdateCommandForTest(store jujuclient.ClientStore, api UpdateSecretsAPI) *updateSecretCommand {
	c := &updateSecretCommand{
		secretsAPIFunc: func(ctx context.Context) (UpdateSecretsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewRemoveCommandForTest returns a secrets command for testing.
func NewRemoveCommandForTest(store jujuclient.ClientStore, api RemoveSecretsAPI) *removeSecretCommand {
	c := &removeSecretCommand{
		secretsAPIFunc: func(ctx context.Context) (RemoveSecretsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewGrantCommandForTest returns a secrets command for testing.
func NewGrantCommandForTest(store jujuclient.ClientStore, api GrantRevokeSecretsAPI) *grantSecretCommand {
	c := &grantSecretCommand{
		secretsAPIFunc: func(ctx context.Context) (GrantRevokeSecretsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewRevokeCommandForTest returns a secrets command for testing.
func NewRevokeCommandForTest(store jujuclient.ClientStore, api GrantRevokeSecretsAPI) *revokeSecretCommand {
	c := &revokeSecretCommand{
		secretsAPIFunc: func(ctx context.Context) (GrantRevokeSecretsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewListCommandForTest returns a secrets command for testing.
func NewListCommandForTest(store jujuclient.ClientStore, listSecretsAPI ListSecretsAPI) *listSecretsCommand {
	c := &listSecretsCommand{
		listSecretsAPIFunc: func(ctx context.Context) (ListSecretsAPI, error) { return listSecretsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewShowCommandForTest returns a list-secrets command for testing.
func NewShowCommandForTest(store jujuclient.ClientStore, listSecretsAPI ListSecretsAPI) *showSecretsCommand {
	c := &showSecretsCommand{
		listSecretsAPIFunc: func(ctx context.Context) (ListSecretsAPI, error) { return listSecretsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}
