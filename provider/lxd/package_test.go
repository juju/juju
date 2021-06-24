// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package lxd -destination package_mock_test.go github.com/juju/juju/provider/lxd Server,ServerFactory,InterfaceAddress,CertificateReadWriter,CertificateGenerator,NetLookup,LXCConfigReader

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
