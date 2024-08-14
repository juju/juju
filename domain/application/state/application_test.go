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
	"github.com/juju/juju/core/network"
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

func (s *applicationStateSuite) createApplication(c *gc.C, name string, l life.Life, units ...application.UpsertUnitArg) coreapplication.ID {
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
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
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
	scale := application.ScaleState{Scale: 0}
	s.assertApplication(c, "666", platform, scale, channel)
}

func (s *applicationStateSuite) TestCreateApplication(c *gc.C) {
	u := application.UpsertUnitArg{
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

func (s *applicationStateSuite) TestApplicationLife(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Dying)
	var appLife life.Life
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		var err error
		appLife, err = stateOps.ApplicationLife(ctx, appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appLife, gc.Equals, life.Dying)
}

func (s *applicationStateSuite) TestApplicationLifeNotFound(c *gc.C) {
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		_, err := stateOps.ApplicationLife(ctx, coreapplication.ID(uuid.MustNewUUID().String()))
		return err
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUpsertCloudService(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
	var providerID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT provider_id FROM cloud_service WHERE application_uuid = ?", appID).Scan(&providerID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerID, gc.Equals, "provider-id")
	err = s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationStateSuite) TestUpsertCloudServiceNotFound(c *gc.C) {
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestCreateUnitCloudContainer(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
		CloudContainer: &application.CloudContainer{
			ProviderId: ptr("some-id"),
		},
	}
	s.createApplication(c, "foo", life.Alive, u)

	var (
		providerId string
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
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

func (s *applicationStateSuite) TestUpsertUnit(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
		CloudContainer: &application.CloudContainer{
			ProviderId: ptr("some-id"),
		},
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	u = application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
		CloudContainer: &application.CloudContainer{
			ProviderId: ptr("another-id"),
		},
	}
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		return stateOps.UpsertUnit(ctx, appID, u)
	})
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
	c.Assert(providerId, gc.Equals, "another-id")
}

func (s *applicationStateSuite) TestUnitLife(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	s.createApplication(c, "foo", life.Alive, u)

	var unitLife life.Life
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		var err error
		unitLife, err = stateOps.UnitLife(ctx, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitLife, gc.Equals, life.Alive)
}

func (s *applicationStateSuite) TestUnitLifeNotFound(c *gc.C) {
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		_, err := stateOps.UnitLife(ctx, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIs, uniterrors.NotFound)
}

func (s *applicationStateSuite) TestApplicationScaleState(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	s.createApplication(c, "foo", life.Alive, u)

	scaleState, err := s.state.ApplicationScaleState(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scaleState, jc.DeepEquals, application.ScaleState{
		Scale: 1,
	})
}

func (s *applicationStateSuite) TestApplicationScaleStateOp(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	var scaleState application.ScaleState
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		var err error
		scaleState, err = stateOps.ApplicationScaleState(ctx, appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scaleState, jc.DeepEquals, application.ScaleState{
		Scale: 1,
	})
}

func (s *applicationStateSuite) TestSetDesiredApplicationScale(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		return stateOps.SetDesiredApplicationScale(ctx, appID, 666, true)
	})
	c.Assert(err, jc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, desired_scale_protected FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.DesiredScaleProtected)
			return errors.Trace(err)
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(got, jc.DeepEquals, want)
	}
	checkResult(application.ScaleState{
		Scale:                 666,
		DesiredScaleProtected: true,
	})

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		return stateOps.SetDesiredApplicationScale(ctx, appID, 668, false)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale: 668,
	})
}

func (s *applicationStateSuite) TestSetApplicationScalingState(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	// Set up the initial scale value.
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		return stateOps.SetDesiredApplicationScale(ctx, appID, 666, false)
	})
	c.Assert(err, jc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.Scaling, &got.ScaleTarget)
			return errors.Trace(err)
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(got, jc.DeepEquals, want)
	}

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		return stateOps.SetApplicationScalingState(ctx, appID, nil, 668, true)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       666,
		ScaleTarget: 668,
		Scaling:     true,
	})

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		stateOps := stateTxOperations{
			ApplicationState: s.state,
			tx:               tx,
		}
		return stateOps.SetApplicationScalingState(ctx, appID, ptr(667), 668, true)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       667,
		ScaleTarget: 668,
		Scaling:     true,
	})
}

func (s *applicationStateSuite) TestExecuteTxnOperation(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	err := s.state.ExecuteTxnOperation(context.Background(), "foo", false, func(ctx context.Context, ops application.StateTxOperations, id coreapplication.ID) error {
		c.Assert(appID, gc.Equals, id)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationStateSuite) TestExecuteTxnOperationNotFound(c *gc.C) {
	err := s.state.ExecuteTxnOperation(context.Background(), "foo", false, func(ctx context.Context, ops application.StateTxOperations, id coreapplication.ID) error {
		c.Fail()
		return nil
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestExecuteTxnOperationDead(c *gc.C) {
	s.createApplication(c, "foo", life.Dead)

	err := s.state.ExecuteTxnOperation(context.Background(), "foo", false, func(ctx context.Context, ops application.StateTxOperations, id coreapplication.ID) error {
		c.Fail()
		return nil
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *applicationStateSuite) TestExecuteTxnOperationDeadOk(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Dead)

	err := s.state.ExecuteTxnOperation(context.Background(), "foo", true, func(ctx context.Context, ops application.StateTxOperations, id coreapplication.ID) error {
		c.Assert(appID, gc.Equals, id)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationStateSuite) TestDeleteApplication(c *gc.C) {
	s.createApplication(c, "foo", life.Alive)

	err := s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	var (
		appCount      int
		platformCount int
		scaleCount    int
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
JOIN application_scale asc ON a.uuid = asc.application_uuid
WHERE a.name=?`,
			"foo").Scan(&scaleCount)
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
	c.Assert(scaleCount, gc.Equals, 0)
	c.Assert(channelCount, gc.Equals, 0)
}

func (s *applicationStateSuite) TestDeleteApplicationTwice(c *gc.C) {
	s.createApplication(c, "foo", life.Alive)

	err := s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestDeleteDeadApplication(c *gc.C) {
	s.createApplication(c, "foo", life.Dead)

	err := s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestDeleteApplicationWithUnits(c *gc.C) {
	u := application.UpsertUnitArg{
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
	s.createApplication(c, "foo", life.Alive)

	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	err := s.state.AddUnits(context.Background(), "foo", u)
	c.Assert(err, jc.ErrorIsNil)

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

func (s *applicationStateSuite) TestAddUnitsDead(c *gc.C) {
	s.createApplication(c, "foo", life.Dead)

	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	err := s.state.AddUnits(context.Background(), "foo", u)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *applicationStateSuite) TestAddUnitsMissingApplication(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	err := s.state.AddUnits(context.Background(), "foo", u)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetUnitLife(c *gc.C) {
	u1 := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	u2 := application.UpsertUnitArg{
		UnitName: ptr("foo/667"),
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	var unitID string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID)
		return errors.Trace(err)
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.state.GetUnitLife(context.Background(), []string{unitID})
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
