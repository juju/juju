// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	stdtesting "testing"

	"github.com/juju/juju/jujuclient"
	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsapi.go github.com/juju/juju/cmd/juju/secrets ListSecretsAPI

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// NewListCommandForTest returns a list-secrets command for testing.
func NewListCommandForTest(store jujuclient.ClientStore, listSecretsAPI ListSecretsAPI) *listSecretsCommand {
	c := &listSecretsCommand{
		listSecretsAPIFunc: func() (ListSecretsAPI, error) { return listSecretsAPI, nil },
	}
	c.SetClientStore(store)
	return c
}
