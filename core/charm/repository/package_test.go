// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/charmhub_client_mock.go github.com/juju/juju/core/charm/repository CharmHubClient
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/logger_mock.go github.com/juju/juju/core/charm/repository Logger

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
