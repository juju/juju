// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	uniterrors "github.com/juju/juju/domain/unit/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type applicationStateSuite struct {
	schematesting.ModelSuite

	state *ApplicationState
}

var _ = gc.Suite(&applicationStateSuite{})

func (s *applicationStateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewApplicationState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())}, loggertesting.WrapCheckLog(c))
}

func (s *applicationStateSuite) assertApplication(
	c *gc.C, name string, platform application.Platform, scale application.ScaleState, channel *applicationcharm.Channel,
) {
	var (
		gotName     string
		gotUUID     string
		gotPlatform application.Platform
		gotScale    application.ScaleState
		gotChannel  applicationcharm.Channel
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid, name FROM application").Scan(&gotUUID, &gotName)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", gotUUID).
			Scan(&gotScale.Scale, &gotScale.Scaling, &gotScale.ScaleTarget)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, "SELECT channel, os_id, architecture_id FROM application_platform WHERE application_uuid=?", gotUUID).
			Scan(&gotPlatform.Channel, &gotPlatform.OSTypeID, &gotPlatform.ArchitectureID)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, "SELECT track, risk, branch FROM application_channel WHERE application_uuid=?", gotUUID).
			Scan(&gotChannel.Track, &gotChannel.Risk, &gotChannel.Branch)
		if err != nil && (channel != nil || !errors.Is(err, sqlair.ErrNoRows)) {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotName, gc.Equals, name)
	c.Assert(gotPlatform, jc.DeepEquals, platform)
	c.Assert(gotScale, jc.DeepEquals, scale)
	if channel != nil {
		c.Assert(gotChannel, jc.DeepEquals, *channel)
	}
}

func (s *applicationStateSuite) TestCreateApplicationNoUnits(c *gc.C) {
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	channel := &applicationcharm.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 0}
	s.assertApplication(c, "666", platform, scale, channel)
}

func (s *applicationStateSuite) TestCreateApplication(c *gc.C) {
	u := application.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	channel := &applicationcharm.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	}, u)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, scale, channel)

	var unitID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM unit").Scan(&unitID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitID, gc.Equals, "foo/666")
}

func (s *applicationStateSuite) TestCreateUnitCloudContainer(c *gc.C) {
	u := application.AddUnitArg{
		UnitName: ptr("foo/666"),
		CloudContainer: &application.CloudContainer{
			ProviderId: ptr("some-id"),
		},
	}
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	channel := &applicationcharm.Channel{
		Risk: "risk",
	}
	ctx := context.Background()
	_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	}, u)
	c.Assert(err, jc.ErrorIsNil)

	var (
		providerId string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `
SELECT provider_id FROM cloud_container cc
JOIN unit u ON cc.net_node_uuid = u.net_node_uuid
WHERE u.name=?`,
			"foo/666").Scan(&providerId)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerId, gc.Equals, "some-id")
}

func (s *applicationStateSuite) TestUnitLife(c *gc.C) {
	u := application.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	channel := &applicationcharm.Channel{
		Risk: "risk",
	}
	ctx := context.Background()
	_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	}, u)
	c.Assert(err, jc.ErrorIsNil)

	var unitLife life.Life
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		unitLife, err = s.state.UnitLife(ctx, tx, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitLife, gc.Equals, life.Alive)
}

func (s *applicationStateSuite) TestUnitLifeNotFound(c *gc.C) {
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := s.state.UnitLife(ctx, tx, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIs, uniterrors.NotFound)
}

func (s *applicationStateSuite) TestApplicationScaleState(c *gc.C) {
	u := application.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	channel := &applicationcharm.Channel{
		Risk: "risk",
	}
	ctx := context.Background()
	appID, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	}, u)
	c.Assert(err, jc.ErrorIsNil)

	var scaleState application.ScaleState
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		scaleState, err = s.state.ApplicationScaleState(ctx, tx, appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scaleState, jc.DeepEquals, application.ScaleState{
		Scale: 1,
	})
}

func (s *applicationStateSuite) TestApplicationScaleStateNotFOund(c *gc.C) {
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := s.state.ApplicationScaleState(ctx, tx, coreapplication.ID(uuid.MustNewUUID().String()))
		return err
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUpdateCAASApplicationUnit(c *gc.C) {
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	appID, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.UpsertCAASApplicationUnit(context.Background(), "666", func(ctx context.Context, tx *sqlair.TX, id coreapplication.ID) error {
		c.Assert(appID, gc.Equals, id)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationStateSuite) TestUpdateCAASApplicationUnitNotFound(c *gc.C) {
	err := s.state.UpsertCAASApplicationUnit(context.Background(), "666", func(ctx context.Context, tx *sqlair.TX, id coreapplication.ID) error {
		c.Fail()
		return nil
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestDeleteApplication(c *gc.C) {
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	channel := &applicationcharm.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.DeleteApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	var (
		appCount      int
		platformCount int
		scaleCount    int
		channelCount  int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name=?", "666").Scan(&appCount)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, `
SELECT count(*) FROM application a
JOIN application_platform ap ON a.uuid = ap.application_uuid
WHERE a.name=?`,
			"666").Scan(&platformCount)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, `
SELECT count(*) FROM application a
JOIN application_scale asc ON a.uuid = asc.application_uuid
WHERE a.name=?`,
			"666").Scan(&scaleCount)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, `
SELECT count(*) FROM application a
JOIN application_channel ac ON a.uuid = ac.application_uuid
WHERE a.name=?`,
			"666").Scan(&channelCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appCount, gc.Equals, 0)
	c.Assert(platformCount, gc.Equals, 0)
	c.Assert(scaleCount, gc.Equals, 0)
	c.Assert(channelCount, gc.Equals, 0)
}

func (s *applicationStateSuite) TestDeleteApplicationTwice(c *gc.C) {
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.DeleteApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.DeleteApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestDeleteApplicationWithUnits(c *gc.C) {
	u := application.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	}, u)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.DeleteApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationHasUnits)
	c.Assert(err, gc.ErrorMatches, `.*cannot delete application "666" as it still has 1 unit\(s\)`)

	var appCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name=?", "666").Scan(&appCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appCount, gc.Equals, 1)
}

func (s *applicationStateSuite) TestAddUnits(c *gc.C) {
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: "666",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	u := application.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	err = s.state.AddUnits(context.Background(), "666", u)
	c.Assert(err, jc.ErrorIsNil)

	scale := application.ScaleState{Scale: 0}
	s.assertApplication(c, "666", platform, scale, nil)

	var unitID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM unit").Scan(&unitID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitID, gc.Equals, "foo/666")
}

func (s *applicationStateSuite) TestAddUnitsMissingApplication(c *gc.C) {
	u := application.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	err := s.state.AddUnits(context.Background(), "666", u)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestStorageDefaultsNone(c *gc.C) {
	defaults, err := s.state.StorageDefaults(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, domainstorage.StorageDefaults{})
}

func (s *applicationStateSuite) TestStorageDefaults(c *gc.C) {
	db := s.DB()
	_, err := db.ExecContext(context.Background(), "INSERT INTO model_config (key, value) VALUES (?, ?)",
		"storage-default-block-source", "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.ExecContext(context.Background(), "INSERT INTO model_config (key, value) VALUES (?, ?)",
		"storage-default-filesystem-source", "elastic-fs")
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := s.state.StorageDefaults(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, domainstorage.StorageDefaults{
		DefaultBlockSource:      ptr("ebs-fast"),
		DefaultFilesystemSource: ptr("elastic-fs"),
	})
}
