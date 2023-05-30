// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package externalcontrollerupdater_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater EcService

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
