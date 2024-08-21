// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
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

	s.state = NewApplicationState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

func (s *applicationStateSuite) TestCreateApplicationNoUnits(c *gc.C) {
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	origin := charm.CharmOrigin{
		Source:   charm.CharmHubSource,
		Revision: 42,
		Channel: &charm.Channel{
			Track:  "track",
			Risk:   "risk",
			Branch: "branch",
		},
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Origin:   origin,
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: "666",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 0}
	s.assertApplication(c, "666", platform, scale, origin)
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
	origin := charm.CharmOrigin{
		Source:   charm.LocalSource,
		Revision: 42,
		Channel: &charm.Channel{
			Track:  "track",
			Risk:   "risk",
			Branch: "branch",
		},
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Origin:   origin,
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: "666",
			},
		},
	}, u)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, scale, origin)

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

func (s *applicationStateSuite) TestCreateApplicationWithoutChannel(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	origin := charm.CharmOrigin{
		Source:   charm.LocalSource,
		Revision: 42,
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Origin:   origin,
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: "666",
			},
		},
	}, u)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, scale, origin)

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

func (s *applicationStateSuite) TestCreateApplicationWithEmptyChannel(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	platform := application.Platform{
		Channel:        "666",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	origin := charm.CharmOrigin{
		Source:   charm.LocalSource,
		Revision: 42,
		Channel:  &charm.Channel{},
	}
	_, err := s.state.CreateApplication(context.Background(), "666", application.AddApplicationArg{
		Platform: platform,
		Origin:   origin,
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: "666",
			},
		},
	}, u)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, scale, origin)

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

func (s *applicationStateSuite) TestGetApplicationLife(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Dying)
	var (
		appLife life.Life
		gotID   coreapplication.ID
	)
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		gotID, appLife, err = s.state.GetApplicationLife(ctx, "foo")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotID, gc.Equals, appID)
	c.Assert(appLife, gc.Equals, life.Dying)
}

func (s *applicationStateSuite) TestGetApplicationLifeNotFound(c *gc.C) {
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, _, err := s.state.GetApplicationLife(ctx, "foo")
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
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.UpsertUnit(ctx, appID, u)
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

func (s *applicationStateSuite) TestGetUnitLife(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	s.createApplication(c, "foo", life.Alive, u)

	var unitLife life.Life
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		unitLife, err = s.state.GetUnitLife(ctx, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitLife, gc.Equals, life.Alive)
}

func (s *applicationStateSuite) TestGetUnitLifeNotFound(c *gc.C) {
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.GetUnitLife(ctx, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestSetUnitLife(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	s.createApplication(c, "foo", life.Alive, u)

	checkResult := func(want life.Life) {
		var gotLife life.Life
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT life_id FROM unit WHERE name=?", u.UnitName).
				Scan(&gotLife)
			return errors.Trace(err)
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotLife, jc.DeepEquals, want)
	}

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dying)

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitLife(ctx, "foo/666", life.Dead)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)

	// Can't go backwards.
	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)
}

func (s *applicationStateSuite) TestSetUnitLifeNotFound(c *gc.C) {
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestDeleteUnit(c *gc.C) {
	u1 := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	u2 := application.UpsertUnitArg{
		UnitName: ptr("foo/667"),
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	err := s.state.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var unitCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).Scan(&unitCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitCount, gc.Equals, 0)
}

func (s *applicationStateSuite) TestRemoveUnitMaybeApplicationUnitsRemaining(c *gc.C) {
	u1 := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	u2 := application.UpsertUnitArg{
		UnitName: ptr("foo/667"),
	}
	appID := s.createApplication(c, "foo", life.Dying, u1, u2)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName)
		return errors.Trace(err)
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.RemoveUnitMaybeApplication(ctx, "foo/666")
	})
	c.Assert(err, jc.ErrorIsNil)

	var (
		unitCount int
		appCount  int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).
			Scan(&unitCount); err != nil {
			return errors.Trace(err)
		}
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE uuid=?", appID).
			Scan(&appCount); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitCount, gc.Equals, 0)
	c.Assert(appCount, gc.Equals, 1)
}

func (s *applicationStateSuite) TestRemoveUnitMaybeApplicationLastUnitAppAlive(c *gc.C) {
	u1 := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	appID := s.createApplication(c, "foo", life.Alive, u1)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.RemoveUnitMaybeApplication(ctx, "foo/666")
	})
	c.Assert(err, jc.ErrorIsNil)

	var (
		unitCount int
		appCount  int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).
			Scan(&unitCount); err != nil {
			return errors.Trace(err)
		}
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE uuid=?", appID).
			Scan(&appCount); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitCount, gc.Equals, 0)
	c.Assert(appCount, gc.Equals, 1)
}

func (s *applicationStateSuite) TestRemoveUnitMaybeApplicationLastUnit(c *gc.C) {
	u1 := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	appID := s.createApplication(c, "foo", life.Dying, u1)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.RemoveUnitMaybeApplication(ctx, "foo/666")
	})
	c.Assert(err, jc.ErrorIsNil)

	var (
		unitCount int
		appCount  int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).
			Scan(&unitCount); err != nil {
			return errors.Trace(err)
		}
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE uuid=?", appID).
			Scan(&appCount); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitCount, gc.Equals, 0)
	c.Assert(appCount, gc.Equals, 0)
}

func (s *applicationStateSuite) TestGetApplicationScaleState(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	var scaleState application.ScaleState
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		scaleState, err = s.state.GetApplicationScaleState(ctx, appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scaleState, jc.DeepEquals, application.ScaleState{
		Scale: 1,
	})
}

func (s *applicationStateSuite) TestGetApplicationScaleStateNotFound(c *gc.C) {
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.GetApplicationScaleState(ctx, coreapplication.ID(uuid.MustNewUUID().String()))
		return err
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetDesiredApplicationScale(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetDesiredApplicationScale(ctx, appID, 666)
	})
	c.Assert(err, jc.ErrorIsNil)

	var gotScale int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid=?", appID).
			Scan(&gotScale)
		return errors.Trace(err)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScale, jc.DeepEquals, 666)
}

func (s *applicationStateSuite) TestSetApplicationScalingState(c *gc.C) {
	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	// Set up the initial scale value.
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetDesiredApplicationScale(ctx, appID, 666)
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

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationScalingState(ctx, appID, nil, 668, true)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       666,
		ScaleTarget: 668,
		Scaling:     true,
	})

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationScalingState(ctx, appID, ptr(667), 668, true)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       667,
		ScaleTarget: 668,
		Scaling:     true,
	})
}

func (s *applicationStateSuite) TestSetApplicationLife(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	checkResult := func(want life.Life) {
		var gotLife life.Life
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT life_id FROM application WHERE uuid=?", appID).
				Scan(&gotLife)
			return errors.Trace(err)
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotLife, jc.DeepEquals, want)
	}

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationLife(ctx, appID, life.Dying)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dying)

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationLife(ctx, appID, life.Dead)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)

	// Can't go backwards.
	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationLife(ctx, appID, life.Dying)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)
}

func (s *applicationStateSuite) TestDeleteApplication(c *gc.C) {
	s.createApplication(c, "foo", life.Alive)

	err := s.state.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	var (
		appCount      int
		platformCount int
		originCount   int
		channelCount  int
		scaleCount    int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name=?", "foo").Scan(&appCount)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, `
SELECT count(*) FROM application a
JOIN v_application_platform ap ON a.uuid = ap.application_uuid
WHERE a.name=?`,
			"foo").Scan(&platformCount)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, `
SELECT count(*) FROM application a
JOIN v_application_origin ap ON a.uuid = ap.application_uuid
WHERE a.name=?`,
			"666").Scan(&originCount)
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
			"foo").Scan(&channelCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appCount, gc.Equals, 0)
	c.Check(platformCount, gc.Equals, 0)
	c.Check(originCount, gc.Equals, 0)
	c.Check(channelCount, gc.Equals, 0)
	c.Check(scaleCount, gc.Equals, 0)
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
	c.Check(appCount, gc.Equals, 1)
}

func (s *applicationStateSuite) TestAddUnits(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	u := application.UpsertUnitArg{
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
	c.Check(unitID, gc.Equals, "foo/666")
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

func (s *applicationStateSuite) TestGetApplicationUnitLife(c *gc.C) {
	u1 := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	u2 := application.UpsertUnitArg{
		UnitName: ptr("foo/667"),
	}
	u3 := application.UpsertUnitArg{
		UnitName: ptr("bar/667"),
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)
	s.createApplication(c, "bar", life.Alive, u3)

	var unitID1, unitID2, unitID3 string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name=?", "foo/666"); err != nil {
			return errors.Trace(err)
		}
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&unitID1); err != nil {
			return errors.Trace(err)
		}
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2); err != nil {
			return errors.Trace(err)
		}
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "bar/667").Scan(&unitID3)
		return errors.Trace(err)
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.state.GetApplicationUnitLife(context.Background(), "foo", unitID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string]life.Life{
		unitID2: life.Alive,
	})

	got, err = s.state.GetApplicationUnitLife(context.Background(), "foo", unitID1, unitID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string]life.Life{
		unitID1: life.Dead,
		unitID2: life.Alive,
	})

	got, err = s.state.GetApplicationUnitLife(context.Background(), "foo", unitID2, unitID3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string]life.Life{
		unitID2: life.Alive,
	})

	got, err = s.state.GetApplicationUnitLife(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 0)
}

func (s *applicationStateSuite) TestInitialWatchStatementUnitLife(c *gc.C) {
	u1 := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	u2 := application.UpsertUnitArg{
		UnitName: ptr("foo/667"),
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	var unitID1, unitID2 string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&unitID1); err != nil {
			return errors.Trace(err)
		}
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2)
		return errors.Trace(err)
	})
	c.Assert(err, jc.ErrorIsNil)

	table, queryFunc := s.state.InitialWatchStatementUnitLife("foo")
	c.Assert(table, gc.Equals, "unit")

	result, err := queryFunc(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []string{unitID1, unitID2})
}

func (s *applicationStateSuite) TestStorageDefaultsNone(c *gc.C) {
	defaults, err := s.state.StorageDefaults(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, domainstorage.StorageDefaults{})
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
	c.Check(defaults, jc.DeepEquals, domainstorage.StorageDefaults{
		DefaultBlockSource:      ptr("ebs-fast"),
		DefaultFilesystemSource: ptr("elastic-fs"),
	})
}

func (s *applicationStateSuite) TestSetCharmThenGetCharmByApplicationName(c *gc.C) {
	origin := charm.CharmOrigin{
		Revision: 42,
	}

	expectedMetadata := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}
	expectedManifest := charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track: "latest",
					Risk:  charm.RiskEdge,
				},
				Architectures: []string{"amd64", "arm64"},
			},
		},
	}
	expectedActions := charm.Actions{
		Actions: map[string]charm.Action{
			"action1": {
				Description:    "description",
				Parallel:       true,
				ExecutionGroup: "group",
				Params:         []byte(`{}`),
			},
		},
	}
	expectedConfig := charm.Config{
		Options: map[string]charm.Option{
			"option1": {
				Type:        "string",
				Description: "description",
				Default:     "default",
			},
		},
	}
	expectedLXDProfile := []byte("[{}]")

	revision := 42
	_, err := s.state.CreateApplication(context.Background(), "foo", application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata:   expectedMetadata,
			Manifest:   expectedManifest,
			Actions:    expectedActions,
			Config:     expectedConfig,
			LXDProfile: expectedLXDProfile,
		},
		Origin: origin,
	})
	c.Assert(err, jc.ErrorIsNil)

	ch, info, err := s.state.GetCharmByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, charm.Charm{
		Metadata:   expectedMetadata,
		Manifest:   expectedManifest,
		Actions:    expectedActions,
		Config:     expectedConfig,
		LXDProfile: expectedLXDProfile,
	})
	c.Check(info, gc.DeepEquals, charm.CharmOrigin{
		Source:   charm.CharmHubSource,
		Revision: revision,
	})
}

func (s *applicationStateSuite) TestSetCharmThenGetCharmByApplicationNameInvalidName(c *gc.C) {
	expectedMetadata := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}
	expectedManifest := charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track: "latest",
					Risk:  charm.RiskEdge,
				},
				Architectures: []string{"amd64", "arm64"},
			},
		},
	}
	expectedActions := charm.Actions{
		Actions: map[string]charm.Action{
			"action1": {
				Description:    "description",
				Parallel:       true,
				ExecutionGroup: "group",
				Params:         []byte(`{}`),
			},
		},
	}
	expectedConfig := charm.Config{
		Options: map[string]charm.Option{
			"option1": {
				Type:        "string",
				Description: "description",
				Default:     "default",
			},
		},
	}
	expectedLXDProfile := []byte("[{}]")

	_, err := s.state.CreateApplication(context.Background(), "foo", application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata:   expectedMetadata,
			Manifest:   expectedManifest,
			Actions:    expectedActions,
			Config:     expectedConfig,
			LXDProfile: expectedLXDProfile,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.state.GetCharmByApplicationName(context.Background(), "bar")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) assertApplication(
	c *gc.C,
	name string,
	platform application.Platform,
	scale application.ScaleState,
	origin charm.CharmOrigin,
) {
	var (
		gotName     string
		gotUUID     string
		gotPlatform application.Platform
		gotScale    application.ScaleState
		gotOrigin   charm.CharmOrigin
		gotChannel  dbChannel
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
		err = tx.QueryRowContext(ctx, "SELECT channel, os_id, architecture_id FROM v_application_platform WHERE application_uuid=?", gotUUID).
			Scan(&gotPlatform.Channel, &gotPlatform.OSTypeID, &gotPlatform.ArchitectureID)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.QueryRowContext(ctx, "SELECT source_name, revision, track, risk, branch FROM v_application_origin WHERE application_uuid=?", gotUUID).
			Scan(&gotOrigin.Source, &gotOrigin.Revision, &gotChannel.Track, &gotChannel.Risk, &gotChannel.Branch)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotName, gc.Equals, name)
	c.Check(gotPlatform, jc.DeepEquals, platform)
	c.Check(gotScale, jc.DeepEquals, scale)

	c.Check(gotOrigin.Revision, gc.Equals, origin.Revision)
	c.Check(gotOrigin.Source, gc.Equals, origin.Source)

	// Channel is optional, so we need to check it separately.
	if origin.Channel != nil {
		c.Check(gotChannel.toCharmChannel(), gc.DeepEquals, origin.Channel)
	} else {
		// Ensure it's empty if the original origin channel isn't set.
		// Prevent the db from sending back bogus values.
		c.Check(gotChannel, jc.DeepEquals, dbChannel{})
	}
}

func (s *applicationStateSuite) createApplication(c *gc.C, name string, l life.Life, units ...application.UpsertUnitArg) coreapplication.ID {
	platform := application.Platform{
		Channel:        name,
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	appID, err := s.state.CreateApplication(context.Background(), name, application.AddApplicationArg{
		Platform: platform,
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: name,
			},
		},
		Origin: charm.CharmOrigin{
			Source:   charm.CharmHubSource,
			Revision: 42,
			Channel: &charm.Channel{
				Track:  "track",
				Risk:   "risk",
				Branch: "branch",
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

type dbChannel struct {
	Track  *string
	Risk   *string
	Branch *string
}

func (c dbChannel) toCharmChannel() *charm.Channel {
	var (
		track  string
		risk   charm.ChannelRisk
		branch string
	)
	if c.Track != nil {
		track = *c.Track
	}
	if c.Risk != nil {
		risk = charm.ChannelRisk(*c.Risk)
	}
	if c.Branch != nil {
		branch = *c.Branch
	}
	return &charm.Channel{
		Track:  track,
		Risk:   risk,
		Branch: branch,
	}
}
