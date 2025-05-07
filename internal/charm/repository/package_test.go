// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/charmhub_client_mock.go github.com/juju/juju/internal/charm/repository CharmHubClient

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
