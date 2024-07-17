// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package secretbackends -destination secretbackendsapi_mock_test.go github.com/juju/juju/cmd/juju/secretbackends ListSecretBackendsAPI,AddSecretBackendsAPI,RemoveSecretBackendsAPI,UpdateSecretBackendsAPI,ModelSecretBackendAPI

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// NewListCommandForTest returns a secret backends command for testing.
func NewListCommandForTest(store jujuclient.ClientStore, listSecretsAPI ListSecretBackendsAPI) *listSecretBackendsCommand {
	c := &listSecretBackendsCommand{
		listSecretBackendsAPIFunc: func() (ListSecretBackendsAPI, error) { return listSecretsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewShowCommandForTest returns a show secret backend command for testing.
func NewShowCommandForTest(store jujuclient.ClientStore, showSecretsAPI ListSecretBackendsAPI) *showSecretBackendCommand {
	c := &showSecretBackendCommand{
		ShowSecretBackendsAPIFunc: func() (ShowSecretBackendsAPI, error) { return showSecretsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewAddCommandForTest returns an add secret backends command for testing.
func NewAddCommandForTest(store jujuclient.ClientStore, addSecretBackendsAPI AddSecretBackendsAPI) *addSecretBackendCommand {
	c := &addSecretBackendCommand{
		AddSecretBackendsAPIFunc: func() (AddSecretBackendsAPI, error) { return addSecretBackendsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewRemoveCommandForTest returns a remove secret backends command for testing.
func NewRemoveCommandForTest(store jujuclient.ClientStore, removeSecretBackendsAPI RemoveSecretBackendsAPI) *removeSecretBackendCommand {
	c := &removeSecretBackendCommand{
		RemoveSecretBackendsAPIFunc: func() (RemoveSecretBackendsAPI, error) { return removeSecretBackendsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewUpdateCommandForTest returns a remove secret backends command for testing.
func NewUpdateCommandForTest(store jujuclient.ClientStore, updateSecretBackendsAPI UpdateSecretBackendsAPI) *updateSecretBackendCommand {
	c := &updateSecretBackendCommand{
		UpdateSecretBackendsAPIFunc: func() (UpdateSecretBackendsAPI, error) { return updateSecretBackendsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewModelCredentialCommandForTest returns a model secret backend command for testing.
func NewModelCredentialCommandForTest(store jujuclient.ClientStore, api ModelSecretBackendAPI) *modelSecretBackendCommand {
	c := &modelSecretBackendCommand{
		getAPIFunc: func() (ModelSecretBackendAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}
