// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/errors"
	modeltesting "github.com/juju/juju/core/model/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/apiremoterelationcaller"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/consumerunitrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/offererrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/remoteunitrelations"
)

type manifoldSuite struct {
	baseSuite

	config ManifoldConfig
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *manifoldSuite) TestValidate(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIsNil)

	invalid := s.validConfig(c)
	invalid.ModelUUID = ""
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.APIRemoteRelationCallerName = ""
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.DomainServicesName = ""
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.GetCrossModelServices = nil
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.NewRemoteRelationClientGetter = nil
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.NewWorker = nil
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.NewLocalConsumerWorker = nil
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.NewConsumerUnitRelationsWorker = nil
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.NewOffererUnitRelationsWorker = nil
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.NewOffererRelationsWorker = nil
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.Logger = nil
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	invalid = s.validConfig(c)
	invalid.Clock = nil
	err = invalid.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	getter := map[string]any{
		"api-remote-relation-caller": s.apiRemoteCallerGetter,
	}

	manifold := Manifold(ManifoldConfig{
		APIRemoteRelationCallerName: "api-remote-relation-caller",
		DomainServicesName:          "domain-services",
		ModelUUID:                   s.modelUUID,
		NewRemoteRelationClientGetter: func(acg apiremoterelationcaller.APIRemoteCallerGetter) RemoteRelationClientGetter {
			return s.remoteRelationClientGetter
		},
		NewWorker: func(c Config) (ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewLocalConsumerWorker: func(rac LocalConsumerWorkerConfig) (ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewConsumerUnitRelationsWorker: func(c consumerunitrelations.Config) (consumerunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewOffererUnitRelationsWorker: func(c remoteunitrelations.Config) (remoteunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewOffererRelationsWorker: func(c offererrelations.Config) (offererrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		GetCrossModelServices: func(getter dependency.Getter, domainServicesName string) (CrossModelService, error) {
			return nil, nil
		},
		Clock:  clock.WallClock,
		Logger: s.logger,
	})
	w, err := manifold.Start(c.Context(), dt.StubGetter(getter))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	workertest.CheckAlive(c, w)
}

func (s *manifoldSuite) validConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		ModelUUID:                   modeltesting.GenModelUUID(c),
		APIRemoteRelationCallerName: "api-remote-relation-caller",
		DomainServicesName:          "domain-services",
		GetCrossModelServices: func(getter dependency.Getter, domainServicesName string) (CrossModelService, error) {
			return nil, nil
		},
		NewRemoteRelationClientGetter: func(acg apiremoterelationcaller.APIRemoteCallerGetter) RemoteRelationClientGetter {
			return nil
		},
		NewWorker: func(Config) (ReportableWorker, error) {
			return nil, nil
		},
		NewLocalConsumerWorker: func(rac LocalConsumerWorkerConfig) (ReportableWorker, error) {
			return nil, nil
		},
		NewConsumerUnitRelationsWorker: func(c consumerunitrelations.Config) (consumerunitrelations.ReportableWorker, error) {
			return nil, nil
		},
		NewOffererUnitRelationsWorker: func(c remoteunitrelations.Config) (remoteunitrelations.ReportableWorker, error) {
			return nil, nil
		},
		NewOffererRelationsWorker: func(c offererrelations.Config) (offererrelations.ReportableWorker, error) {
			return nil, nil
		},
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
	}
}
