// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package keyupdater -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/keyupdater KeyUpdaterService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
