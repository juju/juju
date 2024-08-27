// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage/provider"
)

type serviceSuite struct {
	testing.ModelSuite

	svc *service.Service
}

var _ = gc.Suite(&serviceSuite{})

func ptr[T any](v T) *T {
	return &v
}

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.svc = service.NewService(
		state.NewApplicationState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil },
			loggertesting.WrapCheckLog(c),
		),
		state.NewCharmState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }),
		provider.CommonStorageProviders(),
		loggertesting.WrapCheckLog(c),
	)
}

func (s *serviceSuite) createApplication(c *gc.C, name string, units ...service.AddUnitArg) coreapplication.ID {
	ctx := context.Background()
	appID, err := s.svc.CreateApplication(ctx, name, &stubCharm{}, corecharm.Origin{
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, service.AddApplicationArgs{}, units...)
	c.Assert(err, jc.ErrorIsNil)
	return appID
}

func (s *serviceSuite) TestDestroyApplication(c *gc.C) {
	appID := s.createApplication(c, "foo")

	err := s.svc.DestroyApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	var gotLife int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT life_id FROM application WHERE uuid = ?", appID).
			Scan(&gotLife)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotLife, gc.Equals, 1)
}

func (s *serviceSuite) TestEnsureUnitDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	u := service.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	s.createApplication(c, "foo", u)

	revoker := application.NewMockRevoker(ctrl)
	revoker.EXPECT().RevokeLeadership("foo", "foo/666")

	err := s.svc.EnsureUnitDead(context.Background(), "foo/666", revoker)
	c.Assert(err, jc.ErrorIsNil)

	var gotLife int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT life_id FROM unit WHERE name = ?", u.UnitName).
			Scan(&gotLife)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotLife, gc.Equals, 2)
}

func (s *serviceSuite) TestEnsureUnitDeadNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo")

	revoker := application.NewMockRevoker(ctrl)

	err := s.svc.EnsureUnitDead(context.Background(), "foo/666", revoker)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestRemoveUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	u := service.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", u.UnitName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	revoker := application.NewMockRevoker(ctrl)
	revoker.EXPECT().RevokeLeadership("foo", "foo/666")

	err = s.svc.RemoveUnit(context.Background(), "foo/666", revoker)
	c.Assert(err, jc.ErrorIsNil)

	var gotCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name = ?", u.UnitName).
			Scan(&gotCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotCount, gc.Equals, 0)
}

func (s *serviceSuite) TestRemoveUnitStillAlive(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	u := service.AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	s.createApplication(c, "foo", u)

	revoker := application.NewMockRevoker(ctrl)

	err := s.svc.RemoveUnit(context.Background(), "foo/666", revoker)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitIsAlive)
}

func (s *serviceSuite) TestRemoveUnitNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo")

	revoker := application.NewMockRevoker(ctrl)

	err := s.svc.RemoveUnit(context.Background(), "foo/666", revoker)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) assertCAASUnit(c *gc.C, name, passwordHash string) {
	var (
		gotPasswordHash string
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM unit WHERE name = ?", name).Scan(&gotPasswordHash)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotPasswordHash, gc.Equals, passwordHash)
}

func (s *serviceSuite) TestReplaceCAASUnit(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	s.createApplication(c, "foo", u)

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: ptr("passwordhash"),
		ProviderId:   ptr("provider-id"),
		OrderedScale: true,
		OrderedId:    1,
	}
	err := s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCAASUnit(c, "foo/1", "passwordhash")
}

func (s *serviceSuite) TestReplaceDeadCAASUnit(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", u.UnitName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: ptr("passwordhash"),
		ProviderId:   ptr("provider-id"),
		OrderedScale: true,
		OrderedId:    1,
	}
	err = s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *serviceSuite) TestNewCAASUnit(c *gc.C) {
	appID := s.createApplication(c, "foo")

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scale = 2 WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: ptr("passwordhash"),
		ProviderId:   ptr("provider-id"),
		OrderedScale: true,
		OrderedId:    1,
	}
	err = s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCAASUnit(c, "foo/1", "passwordhash")
}

func (s *serviceSuite) TestRegisterCAASUnitExceedsScale(c *gc.C) {
	s.createApplication(c, "foo")

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: ptr("passwordhash"),
		ProviderId:   ptr("provider-id"),
		OrderedScale: true,
		OrderedId:    666,
	}
	err := s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *serviceSuite) TestRegisterCAASUnitExceedsScaleTarget(c *gc.C) {
	appID := s.createApplication(c, "foo")

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scale = 3, scale_target = 1, scaling = true WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: ptr("passwordhash"),
		ProviderId:   ptr("provider-id"),
		OrderedScale: true,
		OrderedId:    2,
	}
	err = s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *serviceSuite) TestSetScalingState(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	appID := s.createApplication(c, "foo", u)

	err := s.svc.SetApplicationScalingState(context.Background(), "foo", 1, true)
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotScaleTarget int
		gotScaling     bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale_target, scaling FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScaleTarget, &gotScaling)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScaleTarget, gc.Equals, 1)
	c.Assert(gotScaling, jc.IsTrue)
}

func (s *serviceSuite) TestSetScalingStateAlreadyScaling(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	appID := s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scaling = true WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.svc.SetApplicationScalingState(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotScaleTarget int
		gotScaling     bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale_target, scaling FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScaleTarget, &gotScaling)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScaleTarget, gc.Equals, 666)
	c.Assert(gotScaling, jc.IsTrue)
}

func (s *serviceSuite) TestSetScalingStateDying(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	appID := s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = 1 WHERE uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.svc.SetApplicationScalingState(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotScaleTarget int
		gotScaling     bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale_target, scaling FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScaleTarget, &gotScaling)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScaleTarget, gc.Equals, 666)
	c.Assert(gotScaling, jc.IsTrue)
}

func (s *serviceSuite) TestSetScalingStateInconsistent(c *gc.C) {
	s.createApplication(c, "foo")

	err := s.svc.SetApplicationScalingState(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIs, applicationerrors.ScalingStateInconsistent)
}

func (s *serviceSuite) TestGetScalingState(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	appID := s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scaling = true WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.svc.SetApplicationScalingState(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.svc.GetApplicationScalingState(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, service.ScalingState{
		Scaling:     true,
		ScaleTarget: 666,
	})
}

func (s *serviceSuite) TestSetScale(c *gc.C) {
	appID := s.createApplication(c, "foo")

	err := s.svc.SetApplicationScale(context.Background(), "foo", 666)
	c.Assert(err, jc.ErrorIsNil)

	var gotScale int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScale)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScale, gc.Equals, 666)
}

func (s *serviceSuite) TestGetScale(c *gc.C) {
	s.createApplication(c, "foo")

	err := s.svc.SetApplicationScale(context.Background(), "foo", 666)
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.svc.GetApplicationScale(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, 666)
}

func (s *serviceSuite) TestChangeScale(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	appID := s.createApplication(c, "foo", u)

	newScale, err := s.svc.ChangeApplicationScale(context.Background(), "foo", 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newScale, gc.Equals, 3)

	var gotScale int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid = ?", appID).Scan(&gotScale)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScale, gc.Equals, 3)
}

func (s *serviceSuite) TestChangeScaleInvalid(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	s.createApplication(c, "foo", u)

	_, err := s.svc.ChangeApplicationScale(context.Background(), "foo", -2)
	c.Assert(err, jc.ErrorIs, applicationerrors.ScaleChangeInvalid)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumLessThanScale(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/0"),
	}
	u2 := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	s.createApplication(c, "foo", u, u2)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	broker := application.NewMockBroker(ctrl)
	broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	willRestart, err := s.svc.CAASUnitTerminating(context.Background(), "foo", 1, broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsTrue)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumGreaterThanScale(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/0"),
	}
	s.createApplication(c, "foo", u)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	broker := application.NewMockBroker(ctrl)
	broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	willRestart, err := s.svc.CAASUnitTerminating(context.Background(), "foo", 666, broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsFalse)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumLessThanDesired(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/0"),
	}
	u2 := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	u3 := service.AddUnitArg{
		UnitName: ptr("foo/2"),
	}
	s.createApplication(c, "foo", u, u2, u3)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	broker := application.NewMockBroker(ctrl)
	broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	err := s.svc.SetApplicationScalingState(context.Background(), "foo", 6, false)
	c.Assert(err, jc.ErrorIsNil)

	willRestart, err := s.svc.CAASUnitTerminating(context.Background(), "foo", 2, broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsTrue)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumGreaterThanDesired(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: ptr("foo/0"),
	}
	u2 := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	u3 := service.AddUnitArg{
		UnitName: ptr("foo/2"),
	}
	s.createApplication(c, "foo", u, u2, u3)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 1,
	}, nil)
	broker := application.NewMockBroker(ctrl)
	broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	err := s.svc.SetApplicationScalingState(context.Background(), "foo", 6, false)
	c.Assert(err, jc.ErrorIsNil)

	willRestart, err := s.svc.CAASUnitTerminating(context.Background(), "foo", 2, broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsFalse)
}
