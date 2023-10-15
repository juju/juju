// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/deltatranslater_mock.go github.com/juju/juju/apiserver DeltaTranslater
//go:generate go run go.uber.org/mock/mockgen -package apiserver_test -destination registration_environs_mock_test.go github.com/juju/juju/environs ConnectorInfo
//go:generate go run go.uber.org/mock/mockgen -package apiserver_test -destination registration_proxy_mock_test.go github.com/juju/juju/proxy Proxier
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/resources_mock.go github.com/juju/juju/state Resources

func Test(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

var (
	SetResource = setResource
)
