// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domaintesting "github.com/juju/juju/domain/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

type charmSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&charmSuite{})

func (s *charmSuite) TestSetCharmWithArchitecture(c *tc.C) {
	service := s.setupService(c)

	// We can't use the architecture from the manifest, as there may not be one.
	// So we should default to Unknown.

	metadata := internalcharm.Meta{
		Name: "foo",
	}
	manifest := internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name: "ubuntu",
			Channel: internalcharm.Channel{
				Risk: internalcharm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}

	_, _, err := service.SetCharm(c.Context(), charm.SetCharmArgs{
		Charm:         internalcharm.NewCharmBase(&metadata, &manifest, nil, nil, nil),
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Hash:          "hash",
		ArchivePath:   "archive",
		Version:       "1.0",
		Architecture:  arch.ARM64,
	})
	c.Assert(err, tc.ErrorIsNil)

	_, locator, _, err := service.GetCharm(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.LocalSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(locator.Architecture, tc.Equals, architecture.ARM64)
}

func (s *charmSuite) TestSetCharmWithoutArchitecture(c *tc.C) {
	service := s.setupService(c)

	// We can't use the architecture from the manifest, as there may not be one.
	// So we should default to Unknown.

	metadata := internalcharm.Meta{
		Name: "foo",
	}
	manifest := internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name: "ubuntu",
			Channel: internalcharm.Channel{
				Risk: internalcharm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}

	_, _, err := service.SetCharm(c.Context(), charm.SetCharmArgs{
		Charm:         internalcharm.NewCharmBase(&metadata, &manifest, nil, nil, nil),
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Hash:          "hash",
		ArchivePath:   "archive",
		Version:       "1.0",
	})
	c.Assert(err, tc.ErrorIsNil)

	_, locator, _, err := service.GetCharm(c.Context(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.LocalSource,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(locator.Architecture, tc.Equals, architecture.Unknown)
}

func (s *charmSuite) setupService(c *tc.C) *service.Service {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return service.NewService(
		state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domaintesting.NoopLeaderEnsurer(),
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}
