// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package apiserver_test -destination registration_environs_mock_test.go github.com/juju/juju/environs ConnectorInfo
//go:generate go run go.uber.org/mock/mockgen -typed -package apiserver_test -destination registration_proxy_mock_test.go github.com/juju/juju/internal/proxy Proxier
//go:generate go run go.uber.org/mock/mockgen -typed -package apiserver_test -destination provider_factory_mock_test.go github.com/juju/juju/core/providertracker ProviderFactory

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type StubDBGetter struct{}

func (s StubDBGetter) GetWatchableDB(namespace string) (changestream.WatchableDB, error) {
	if namespace != "controller" {
		return nil, errors.Errorf(`expected a request for "controller" DB; got %q`, namespace)
	}
	return nil, nil
}

type StubDBDeleter struct{}

func (s StubDBDeleter) DeleteDB(namespace string) error {
	return nil
}
