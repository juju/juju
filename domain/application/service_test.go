// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	machineservice "github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/schema/testing"
	secretstate "github.com/juju/juju/domain/secret/state"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/environs"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testing.ModelSuite

	svc         *service.ProviderService
	secretState *secretstate.State

	caasProvider *MockCAASProvider
}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestGetApplicationLifeByName(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo")

	lifeValue, err := s.svc.GetApplicationLifeByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lifeValue, tc.Equals, life.Alive)

	_, err = s.svc.GetApplicationLifeByName(c.Context(), "bar")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *serviceSuite) TestIsSubordinateApplicationForPrincipal(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appID := s.createApplication(c, "foo")

	subordinate, err := s.svc.IsSubordinateApplication(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subordinate, tc.IsFalse)
}

func (s *serviceSuite) TestIsSubordinateApplicationForSubordinate(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appID := s.createSubordinateApplication(c, "foo")

	subordinate, err := s.svc.IsSubordinateApplication(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subordinate, tc.IsTrue)
}

func (s *serviceSuite) TestIsSubordinateApplicationByNameForPrincipal(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo")

	subordinate, err := s.svc.IsSubordinateApplicationByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subordinate, tc.IsFalse)
}

func (s *serviceSuite) TestIsSubordinateApplicationByNameForSubordinate(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.createSubordinateApplication(c, "foo")

	subordinate, err := s.svc.IsSubordinateApplicationByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subordinate, tc.IsTrue)
}

func (s *serviceSuite) TestSetScalingState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := s.createApplication(c, "foo", service.AddUnitArg{})

	err := s.svc.SetApplicationScalingState(c.Context(), "foo", 1, true)
	c.Assert(err, tc.ErrorIsNil)

	var (
		gotScaleTarget int
		gotScaling     bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale_target, scaling FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScaleTarget, &gotScaling)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotScaleTarget, tc.Equals, 1)
	c.Assert(gotScaling, tc.IsTrue)
}

func (s *serviceSuite) TestSetScalingStateAlreadyScaling(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := s.createApplication(c, "foo", service.AddUnitArg{})

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scaling = true WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.svc.SetApplicationScalingState(c.Context(), "foo", 666, true)
	c.Assert(err, tc.ErrorIsNil)

	var (
		gotScaleTarget int
		gotScaling     bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale_target, scaling FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScaleTarget, &gotScaling)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotScaleTarget, tc.Equals, 666)
	c.Assert(gotScaling, tc.IsTrue)
}

func (s *serviceSuite) TestSetScalingStateDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := s.createApplication(c, "foo", service.AddUnitArg{})

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = 1 WHERE uuid = ?", appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.svc.SetApplicationScalingState(c.Context(), "foo", 666, true)
	c.Assert(err, tc.ErrorIsNil)

	var (
		gotScaleTarget int
		gotScaling     bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale_target, scaling FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScaleTarget, &gotScaling)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotScaleTarget, tc.Equals, 666)
	c.Assert(gotScaling, tc.IsTrue)
}

func (s *serviceSuite) TestSetScalingStateInconsistent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.createApplication(c, "foo")

	err := s.svc.SetApplicationScalingState(c.Context(), "foo", 666, true)
	c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
}

func (s *serviceSuite) TestGetScalingState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := s.createApplication(c, "foo", service.AddUnitArg{})

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scaling = true WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.svc.SetApplicationScalingState(c.Context(), "foo", 666, true)
	c.Assert(err, tc.ErrorIsNil)

	got, err := s.svc.GetApplicationScalingState(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, service.ScalingState{
		Scaling:     true,
		ScaleTarget: 666,
	})
}

func (s *serviceSuite) TestSetScale(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := s.createApplication(c, "foo")

	err := s.svc.SetApplicationScale(c.Context(), "foo", 666)
	c.Assert(err, tc.ErrorIsNil)

	var gotScale int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScale)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotScale, tc.Equals, 666)
}

func (s *serviceSuite) TestGetScale(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.createApplication(c, "foo")

	err := s.svc.SetApplicationScale(c.Context(), "foo", 666)
	c.Assert(err, tc.ErrorIsNil)

	got, err := s.svc.GetApplicationScale(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, 666)
}

func (s *serviceSuite) TestChangeScale(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := s.createApplication(c, "foo", service.AddUnitArg{})

	newScale, err := s.svc.ChangeApplicationScale(c.Context(), "foo", 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newScale, tc.Equals, 3)

	var gotScale int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid = ?", appID).Scan(&gotScale)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotScale, tc.Equals, 3)
}

func (s *serviceSuite) TestChangeScaleInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.createApplication(c, "foo", service.AddUnitArg{})

	_, err := s.svc.ChangeApplicationScale(c.Context(), "foo", -2)
	c.Assert(err, tc.ErrorIs, applicationerrors.ScaleChangeInvalid)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumLessThanScale(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo", service.AddUnitArg{}, service.AddUnitArg{})

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	s.caasProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	willRestart, err := s.svc.CAASUnitTerminating(c.Context(), "foo/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(willRestart, tc.IsTrue)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumGreaterThanScale(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo", service.AddUnitArg{}, service.AddUnitArg{}, service.AddUnitArg{})

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 1,
	}, nil)
	s.caasProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	willRestart, err := s.svc.CAASUnitTerminating(c.Context(), "foo/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(willRestart, tc.IsFalse)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNotAlive(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo", service.AddUnitArg{})

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", "foo/0")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	willRestart, err := s.svc.CAASUnitTerminating(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(willRestart, tc.IsFalse)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumLessThanDesired(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo", service.AddUnitArg{}, service.AddUnitArg{}, service.AddUnitArg{})
	err := s.svc.SetApplicationScalingState(c.Context(), "foo", 6, false)
	c.Assert(err, tc.ErrorIsNil)

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	s.caasProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	willRestart, err := s.svc.CAASUnitTerminating(c.Context(), "foo/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(willRestart, tc.IsTrue)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumGreaterThanDesired(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo", service.AddUnitArg{}, service.AddUnitArg{}, service.AddUnitArg{})
	err := s.svc.SetApplicationScalingState(c.Context(), "foo", 6, false)
	c.Assert(err, tc.ErrorIsNil)

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 1,
	}, nil)
	s.caasProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	willRestart, err := s.svc.CAASUnitTerminating(c.Context(), "foo/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(willRestart, tc.IsFalse)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.caasProvider = NewMockCAASProvider(ctrl)

	s.secretState = secretstate.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c))
	s.svc = service.NewProviderService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domaintesting.NoopLeaderEnsurer(),
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		model.UUID(s.ModelUUID()),
		nil,
		func(ctx context.Context) (service.Provider, error) {
			return serviceProvider{}, nil
		},
		func(ctx context.Context) (service.CAASProvider, error) {
			return s.caasProvider, nil
		},
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid,  name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

func (s *serviceSuite) createApplication(c *tc.C, name string, units ...service.AddUnitArg) coreapplication.ID {
	return s.createApplicationWithCharm(c, name, &stubCharm{}, units...)
}

func (s *serviceSuite) createSubordinateApplication(c *tc.C, name string, units ...service.AddUnitArg) coreapplication.ID {
	return s.createApplicationWithCharm(c, name, &stubCharm{subordinate: true}, units...)
}

func (s *serviceSuite) createApplicationWithCharm(c *tc.C, name string, ch internalcharm.Charm, units ...service.AddUnitArg) coreapplication.ID {
	appID, err := s.svc.CreateCAASApplication(c.Context(), name, ch, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, service.AddApplicationArgs{
		ReferenceName: name,
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:  applicationcharm.ProvenanceDownload,
			DownloadURL: "https://example.com",
		},
	}, units...)
	c.Assert(err, tc.ErrorIsNil)
	return appID
}

type serviceProvider struct {
	service.Provider
	service.CAASProvider
}

func (serviceProvider) PrecheckInstance(ctx context.Context, args environs.PrecheckInstanceParams) error {
	return machineservice.NewNoopProvider().PrecheckInstance(ctx, args)
}

func (serviceProvider) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	return nil, nil
}

func (serviceProvider) Application(string, caas.DeploymentType) caas.Application {
	return nil
}
