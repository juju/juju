// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/deltatranslater_mock.go github.com/juju/juju/apiserver DeltaTranslater
//go:generate go run go.uber.org/mock/mockgen -package apiserver_test -destination registration_environs_mock_test.go github.com/juju/juju/environs ConnectorInfo
//go:generate go run go.uber.org/mock/mockgen -package apiserver_test -destination registration_proxy_mock_test.go github.com/juju/juju/proxy Proxier
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/resources_mock.go github.com/juju/juju/state Resources

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type StubDBGetter struct{}

func (s StubDBGetter) GetDB(name string) (coredatabase.TrackedDB, error) {
	if name != "controller" {
		return nil, errors.Errorf(`expected a request for "controller" DB; got %q`, name)
	}
	return nil, nil
}
