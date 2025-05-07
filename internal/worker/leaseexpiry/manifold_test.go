// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	"go.uber.org/mock/gomock"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/leaseexpiry"
	workertrace "github.com/juju/juju/internal/worker/trace"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite

	store *MockExpiryStore
}

var _ = tc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestInputs(c *tc.C) {
	cfg := s.newManifoldConfig(c)

	c.Check(leaseexpiry.Manifold(cfg).Inputs, tc.DeepEquals, []string{"clock-name", "db-accessor-name", "trace-name"})
}

func (s *manifoldSuite) TestConfigValidate(c *tc.C) {
	validCfg := s.newManifoldConfig(c)

	cfg := validCfg
	cfg.ClockName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = validCfg
	cfg.DBAccessorName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = validCfg
	cfg.TraceName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = validCfg
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = validCfg
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = validCfg
	cfg.NewStore = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStartSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newManifoldConfig(c)

	work, err := leaseexpiry.Manifold(cfg).Start(context.Background(), s.newGetter())
	c.Check(work, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.store = NewMockExpiryStore(ctrl)

	return ctrl
}

func (s *manifoldSuite) newGetter() *dt.Getter {
	return dt.StubGetter(map[string]interface{}{
		"clock-name":       clock.WallClock,
		"db-accessor-name": stubDBGetter{runner: nil},
		"trace-name":       stubTracerGetter{},
	})
}

// newManifoldConfig creates and returns a new ManifoldConfig instance based on
// the supplied arguments.
func (s *manifoldSuite) newManifoldConfig(c *tc.C) leaseexpiry.ManifoldConfig {
	return leaseexpiry.ManifoldConfig{
		ClockName:      "clock-name",
		DBAccessorName: "db-accessor-name",
		TraceName:      "trace-name",
		Logger:         loggertesting.WrapCheckLog(c),
		NewWorker:      func(config leaseexpiry.Config) (worker.Worker, error) { return nil, nil },
		NewStore: func(db coredatabase.DBGetter, logger logger.Logger) lease.ExpiryStore {
			return s.store
		},
	}
}

type stubDBGetter struct {
	runner coredatabase.TxnRunner
}

func (s stubDBGetter) GetDB(name string) (coredatabase.TxnRunner, error) {
	if name != "controller" {
		return nil, errors.Errorf(`expected a request for "controller" DB; got %q`, name)
	}
	return s.runner, nil
}

type stubTracerGetter struct {
	workertrace.TracerGetter
}

func (s stubTracerGetter) GetTracer(context.Context, coretrace.TracerNamespace) (coretrace.Tracer, error) {
	return coretrace.NoopTracer{}, nil
}
