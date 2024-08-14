// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/schema/testing"
	uniterrors "github.com/juju/juju/domain/unit/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	testing.ModelSuite
}

var _ = gc.Suite(&serviceSuite{})

func ptr[T any](v T) *T {
	return &v
}

func (s *serviceSuite) createApplication(c *gc.C, svc *service.Service, name string, units ...service.AddUnitArg) coreapplication.ID {
	ctx := context.Background()
	appID, err := svc.CreateApplication(ctx, name, &stubCharm{}, corecharm.Origin{
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, service.AddApplicationArgs{}, units...)
	c.Assert(err, jc.ErrorIsNil)
	return appID
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
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	s.createApplication(c, svc, "foo", u)

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: ptr("passwordhash"),
		ProviderId:   ptr("provider-id"),
		OrderedScale: true,
		OrderedId:    1,
	}
	err := svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCAASUnit(c, "foo/1", "passwordhash")
}

func (s *serviceSuite) TestReplaceDeadCAASUnit(c *gc.C) {
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	s.createApplication(c, svc, "foo", u)

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
	err = svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *serviceSuite) TestNewCAASUnit(c *gc.C) {
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	appID := s.createApplication(c, svc, "foo")

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
	err = svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCAASUnit(c, "foo/1", "passwordhash")
}

func (s *serviceSuite) TestRegisterCAASUnitExceedsScale(c *gc.C) {
	c.Skip("scale not wired up yet")
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	s.createApplication(c, svc, "foo")

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: ptr("passwordhash"),
		ProviderId:   ptr("provider-id"),
		OrderedScale: true,
		OrderedId:    1,
	}
	err := svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIs, uniterrors.NotAssigned)
}

func (s *serviceSuite) TestRegisterCAASUnitExceedsScaleTarget(c *gc.C) {
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	appID := s.createApplication(c, svc, "foo")

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
	err = svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIs, uniterrors.NotAssigned)
}

func (s *serviceSuite) TestSetScalingState(c *gc.C) {
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	appID := s.createApplication(c, svc, "foo", u)

	err := svc.SetScalingState(context.Background(), "foo", 1, true)
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
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	appID := s.createApplication(c, svc, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scaling = true WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = svc.SetScalingState(context.Background(), "foo", 666, true)
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
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	appID := s.createApplication(c, svc, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = 1 WHERE uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = svc.SetScalingState(context.Background(), "foo", 666, true)
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
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	s.createApplication(c, svc, "foo")

	err := svc.SetScalingState(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIs, applicationerrors.ScalingStateInconsistent)
}

func (s *serviceSuite) TestSetScale(c *gc.C) {
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	appID := s.createApplication(c, svc, "foo")

	err := svc.SetScale(context.Background(), "foo", 666, false)
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotScale          int
		gotScaleProtected bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale, desired_scale_protected FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScale, &gotScaleProtected)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScale, gc.Equals, 666)
	c.Assert(gotScaleProtected, jc.IsFalse)
}

func (s *serviceSuite) TestSetScaleProtectedNoMatch(c *gc.C) {
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	s.createApplication(c, svc, "foo")

	err := svc.SetScale(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIsNil)

	err = svc.SetScale(context.Background(), "foo", 667, false)
	c.Assert(err, jc.ErrorIs, applicationerrors.ScaleChangeInvalid)
}

func (s *serviceSuite) TestSetScaleProtectedNoMatchForce(c *gc.C) {
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	appID := s.createApplication(c, svc, "foo")

	err := svc.SetScale(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIsNil)

	err = svc.SetScale(context.Background(), "foo", 667, true)
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotScale          int
		gotScaleProtected bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale, desired_scale_protected FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScale, &gotScaleProtected)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScale, gc.Equals, 667)
	c.Assert(gotScaleProtected, jc.IsTrue)
}

func (s *serviceSuite) TestChangeScale(c *gc.C) {
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	appID := s.createApplication(c, svc, "foo", u)

	newScale, err := svc.ChangeScale(context.Background(), "foo", 2)
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
	svc := service.NewService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	u := service.AddUnitArg{
		UnitName: ptr("foo/1"),
	}
	s.createApplication(c, svc, "foo", u)

	_, err := svc.ChangeScale(context.Background(), "foo", -2)
	c.Assert(err, jc.ErrorIs, applicationerrors.ScaleChangeInvalid)
}
