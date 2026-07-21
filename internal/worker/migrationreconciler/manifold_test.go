// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationreconciler

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
)

const (
	dbAccessorName     = "db-accessor"
	changeStreamName   = "change-stream"
	domainServicesName = "domain-services"
)

type manifoldSuite struct{}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	cfg := s.newConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	bad := cfg
	bad.DBAccessorName = ""
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)

	bad = cfg
	bad.ChangeStreamName = ""
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)

	bad = cfg
	bad.DomainServicesName = ""
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)

	bad = cfg
	bad.Clock = nil
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)

	bad = cfg
	bad.Logger = nil
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)

	bad = cfg
	bad.NewWorker = nil
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Check(Manifold(s.newConfig(c)).Inputs, tc.DeepEquals, []string{
		dbAccessorName,
		changeStreamName,
		domainServicesName,
	})
}

func (s *manifoldSuite) TestStartMissingChangeStream(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		dbAccessorName:   stubDBAccessor{},
		changeStreamName: dependency.ErrMissing,
	})
	w, err := Manifold(s.newConfig(c)).Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorIs, dependency.ErrMissing)
}

func (s *manifoldSuite) TestStartMissingDBAccessor(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		dbAccessorName: dependency.ErrMissing,
	})
	w, err := Manifold(s.newConfig(c)).Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorIs, dependency.ErrMissing)
}

func (s *manifoldSuite) TestStartSuccess(c *tc.C) {
	var captured Config
	cfg := s.newConfig(c)
	cfg.NewWorker = func(config Config) (worker.Worker, error) {
		captured = config
		return nopWorker{}, nil
	}

	getter := dt.StubGetter(map[string]any{
		dbAccessorName:     stubDBAccessor{},
		changeStreamName:   stubWatchableDBGetter{},
		domainServicesName: stubDomainServicesGetter{},
	})
	w, err := Manifold(cfg).Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)

	c.Check(captured.Service, tc.NotNil)
	c.Check(captured.Abort, tc.NotNil)
	c.Check(captured.Activate, tc.NotNil)
	c.Check(captured.Clock, tc.NotNil)
	c.Check(captured.Logger, tc.NotNil)
}

func (s *manifoldSuite) newConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DBAccessorName:     dbAccessorName,
		ChangeStreamName:   changeStreamName,
		DomainServicesName: domainServicesName,
		Clock:              testclock.NewClock(time.Now()),
		Logger:             loggertesting.WrapCheckLog(c),
		NewWorker:          func(Config) (worker.Worker, error) { return nopWorker{}, nil },
	}
}

// stubDBAccessor satisfies both coredatabase.DBGetter and coredatabase.DBDeleter
// so a single stub can back the DB accessor dependency.
type stubDBAccessor struct{}

func (stubDBAccessor) GetDB(context.Context, string) (coredatabase.TxnRunner, error) {
	return nil, nil
}

func (stubDBAccessor) DeleteDB(string) error { return nil }

type stubWatchableDBGetter struct{}

func (stubWatchableDBGetter) GetWatchableDB(context.Context, string) (changestream.WatchableDB, error) {
	return nil, nil
}

type stubDomainServicesGetter struct{}

func (stubDomainServicesGetter) ServicesForModel(context.Context, coremodel.UUID) (services.DomainServices, error) {
	return nil, nil
}

type nopWorker struct{}

func (nopWorker) Kill()       {}
func (nopWorker) Wait() error { return nil }
