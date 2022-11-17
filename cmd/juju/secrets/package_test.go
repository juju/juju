// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsapi.go github.com/juju/juju/cmd/juju/secrets ListSecretsAPI

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
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
