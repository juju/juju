// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package service -destination state_mock_test.go github.com/juju/juju/domain/credential/service State,WatcherFactory,MachineService
//go:generate go run go.uber.org/mock/mockgen -package service -destination validator_mock_test.go github.com/juju/juju/domain/credential/service CredentialValidator

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
