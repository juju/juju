// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"

	"github.com/juju/juju/domain/upgrade"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/upgrade/service State,WatcherFactory

type baseServiceSuite struct {
	testhelpers.IsolationSuite

	upgradeUUID    upgrade.UUID
	controllerUUID string
}

func (s *baseServiceSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.upgradeUUID = upgrade.UUID(uuid.MustNewUUID().String())
	s.controllerUUID = uuid.MustNewUUID().String()
}
