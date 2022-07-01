// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/state"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretstore.go github.com/juju/juju/state SecretsStore

func NewTestService(backend state.SecretsStore) *secretsService {
	return &secretsService{
		backend: backend,
	}
}
