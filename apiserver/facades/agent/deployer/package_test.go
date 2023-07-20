// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	stdtesting "testing"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coretesting "github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type watchableDB struct {
	database.TxnRunner
	changestream.EventSource
}

type mockLeadershipRevoker struct {
	revoked set.Strings
}

func (s *mockLeadershipRevoker) RevokeLeadership(applicationId, unitId string) error {
	s.revoked.Add(unitId)
	return nil
}
