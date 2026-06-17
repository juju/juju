// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/rpc/params"
)

func TestHarvestMigrationMacaroonSuite(t *stdtesting.T) {
	tc.Run(t, &harvestMigrationMacaroonSuite{})
}

type harvestMigrationMacaroonSuite struct{}

// fakeMacaroonMinter is a test double for migration.MigrationMacaroonMinter.
type fakeMacaroonMinter struct {
	called bool
	macs   []macaroon.Slice
	err    error
}

func (f *fakeMacaroonMinter) CreateMigrationMacaroon(_ context.Context) ([]macaroon.Slice, error) {
	f.called = true
	return f.macs, f.err
}

func (s *harvestMigrationMacaroonSuite) TestSuccess(c *tc.C) {
	m, err := macaroon.New([]byte("root-key"), []byte("id"), "loc", macaroon.V2)
	c.Assert(err, tc.ErrorIsNil)

	minter := &fakeMacaroonMinter{macs: []macaroon.Slice{{m}}}
	targetInfo := &coremigration.TargetInfo{
		User:     "admin",
		Password: "hunter2",
	}

	err = migration.HarvestMigrationMacaroon(c.Context(), targetInfo, minter)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(minter.called, tc.IsTrue)
	c.Assert(targetInfo.Password, tc.Equals, "")
	c.Assert(targetInfo.Macaroons, tc.DeepEquals, []macaroon.Slice{{m}})
}

func (s *harvestMigrationMacaroonSuite) TestNotImplemented(c *tc.C) {
	minter := &fakeMacaroonMinter{
		err: &params.Error{Code: params.CodeNotImplemented, Message: "not implemented"},
	}
	targetInfo := &coremigration.TargetInfo{
		User:     "admin",
		Password: "hunter2",
	}

	err := migration.HarvestMigrationMacaroon(c.Context(), targetInfo, minter)

	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Assert(err, tc.ErrorMatches, ".*target controller is too old.*")
	// Password must NOT be cleared — the error prevents persistence.
	c.Assert(targetInfo.Password, tc.Equals, "hunter2")
	c.Assert(targetInfo.Macaroons, tc.HasLen, 0)
}

func (s *harvestMigrationMacaroonSuite) TestSkippedWhenMacaroonsPresent(c *tc.C) {
	m, err := macaroon.New([]byte("root-key"), []byte("id"), "loc", macaroon.V2)
	c.Assert(err, tc.ErrorIsNil)

	minter := &fakeMacaroonMinter{}
	existing := []macaroon.Slice{{m}}
	targetInfo := &coremigration.TargetInfo{
		User:      "admin",
		Password:  "hunter2",
		Macaroons: existing,
	}

	err = migration.HarvestMigrationMacaroon(c.Context(), targetInfo, minter)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(minter.called, tc.IsFalse, tc.Commentf("minter must not be called when macaroons are already present"))
	c.Assert(targetInfo.Password, tc.Equals, "hunter2")
}

func (s *harvestMigrationMacaroonSuite) TestSkippedWhenTokenPresent(c *tc.C) {
	minter := &fakeMacaroonMinter{}
	targetInfo := &coremigration.TargetInfo{
		User:     "admin",
		Password: "hunter2",
		Token:    "jimm-token",
	}

	err := migration.HarvestMigrationMacaroon(c.Context(), targetInfo, minter)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(minter.called, tc.IsFalse, tc.Commentf("minter must not be called when a token is present"))
	c.Assert(targetInfo.Password, tc.Equals, "hunter2")
}
