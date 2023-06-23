// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/juju/core/changestream"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package apiserver_test -destination registration_environs_mock_test.go github.com/juju/juju/environs ConnectorInfo
//go:generate go run github.com/golang/mock/mockgen -package apiserver_test -destination registration_proxy_mock_test.go github.com/juju/juju/proxy Proxier
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/resources_mock.go github.com/juju/juju/state Resources

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
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
	if namespace == "controller" {
		return errors.Forbiddenf(`cannot delete "controller" DB`)
	}
	return nil
}
