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
	loggertesting "github.com/juju/juju/internal/logger/testing"
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

func (s *applicationStateSuite) assertApplication(c *gc.C, name string, platform application.Platform, channel *applicationcharm.Channel) {
	var (
		gotName     string
		gotUUID     string
		gotPlatform application.Platform
		gotChannel  applicationcharm.Channel
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid, name FROM application").Scan(&gotUUID, &gotName)
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
	if channel != nil {
		c.Assert(gotChannel, jc.DeepEquals, *channel)
	}
}

func (s *applicationStateSuite) createApplication(c *gc.C, name string, l life.Life, units ...application.AddUnitArg) coreapplication.ID {
	platform := application.Platform{
		Channel:        name,
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	channel := &applicationcharm.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	appID, err := s.state.CreateApplication(context.Background(), name, application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: applicationcharm.Charm{
			Metadata: applicationcharm.Metadata{
				Name: name,
			},
		},
	}, units...)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id=? WHERE name=?", l, name)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return appID
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
	s.assertApplication(c, "666", platform, channel)
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
	s.assertApplication(c, "666", platform, channel)

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

func (s *applicationStateSuite) TestUpdateApplication(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	u := application.AddUnitArg{
		UnitName:     ptr("foo/666"),
		PasswordHash: ptr("deadbeef"),
	}
	err := s.state.UpsertApplicationUnit(context.Background(), "foo", u)
	c.Assert(err, jc.ErrorIsNil)

	var unitID, hash string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name, password_hash FROM unit WHERE application_uuid=?", appID).Scan(&unitID, &hash)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitID, gc.Equals, "foo/666")
	c.Assert(hash, gc.Equals, "deadbeef")
}

func (s *applicationStateSuite) TestDeleteApplication(c *gc.C) {
	s.createApplication(c, "foo", life.Alive)

	err := s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	var (
		appCount      int
		platformCount int
		channelCount  int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name=?", "foo").Scan(&appCount)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, `
SELECT count(*) FROM application a
JOIN application_platform ap ON a.uuid = ap.application_uuid
WHERE a.name=?`,
			"foo").Scan(&platformCount)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, `
SELECT count(*) FROM application a
JOIN application_channel ac ON a.uuid = ac.application_uuid
WHERE a.name=?`,
			"foo").Scan(&channelCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appCount, gc.Equals, 0)
	c.Assert(platformCount, gc.Equals, 0)
	c.Assert(channelCount, gc.Equals, 0)
}

func (s *applicationStateSuite) TestDeleteApplicationTwice(c *gc.C) {
	s.createApplication(c, "foo", life.Alive)

	err := s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestDeleteApplicationWithUnits(c *gc.C) {
	u := application.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	s.createApplication(c, "foo", life.Alive, u)

	err := s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationHasUnits)
	c.Assert(err, gc.ErrorMatches, `.*cannot delete application "foo" as it still has 1 unit\(s\)`)

	var appCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name=?", "foo").Scan(&appCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appCount, gc.Equals, 1)
}

func (s *applicationStateSuite) TestAddUnits(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	u := application.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	err := s.state.AddUnits(context.Background(), "foo", u)
	c.Assert(err, jc.ErrorIsNil)

	var unitID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM unit WHERE application_uuid=?", appID).Scan(&unitID)
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
	err := s.state.AddUnits(context.Background(), "foo", u)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetUnitLife(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	u2 := application.AddUnitArg{
		UnitName: ptr("foo/667"),
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	var unitID string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID)
		return errors.Trace(err)
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.state.ApplicationUnitLife(context.Background(), "foo", []string{unitID})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string]life.Life{
		unitID: life.Alive,
	})
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
