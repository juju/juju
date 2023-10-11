// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	jujutesting "github.com/juju/testing"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/upgrade"
)

//go:generate go run go.uber.org/mock/mockgen -package service -destination package_mock_test.go github.com/juju/juju/domain/upgrade/service State,WatcherFactory

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseServiceSuite struct {
	jujutesting.IsolationSuite

	upgradeUUID    upgrade.UUID
	controllerUUID string
}

func (s *baseServiceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.upgradeUUID = upgrade.UUID(utils.MustNewUUID().String())
	s.controllerUUID = utils.MustNewUUID().String()
}
