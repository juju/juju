// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsinkproxy

import (
	"context"
	"testing"

	"github.com/juju/errors"
	jc "github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/agent/engine"
	corelogger "github.com/juju/juju/core/logger"
	model "github.com/juju/juju/core/model"
	internaltesting "github.com/juju/juju/internal/testing"
)

type ManifoldSuite struct {
	internaltesting.BaseSuite
}

func TestManifoldSuite(t *testing.T) {
	jc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) TestValidateConfig(c *jc.C) {
	cfg := ManifoldConfig{
		ControllerFlagName:       "is-controller-flag",
		ControllerLogSinkName:    "controller-log-sink",
		NonControllerLogSinkName: "non-controller-log-sink",
	}
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.ControllerFlagName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = ManifoldConfig{
		ControllerFlagName:       "is-controller-flag",
		ControllerLogSinkName:    "",
		NonControllerLogSinkName: "non-controller-log-sink",
	}
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = ManifoldConfig{
		ControllerFlagName:       "is-controller-flag",
		ControllerLogSinkName:    "controller-log-sink",
		NonControllerLogSinkName: "",
	}
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestInputs(c *jc.C) {
	manifold := Manifold(ManifoldConfig{
		ControllerFlagName:       "is-controller-flag",
		ControllerLogSinkName:    "controller-log-sink",
		NonControllerLogSinkName: "non-controller-log-sink",
	})
	c.Check(manifold.Inputs, jc.SameContents, []string{
		"is-controller-flag",
		"controller-log-sink",
		"non-controller-log-sink",
	})
}

func (s *ManifoldSuite) TestOutput(c *jc.C) {
	stub := &stubLogSink{}
	vw, err := engine.NewValueWorker(logSinkProxy{
		ModelLogger:         stub,
		LoggerContextGetter: stub,
		ModelLogSinkGetter:  stub,
	})
	c.Assert(err, jc.ErrorIsNil)

	var ml corelogger.ModelLogger
	err = outputFunc(vw, &ml)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ml, jc.NotNil)

	var lcg corelogger.LoggerContextGetter
	err = outputFunc(vw, &lcg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(lcg, jc.NotNil)

	var msg corelogger.ModelLogSinkGetter
	err = outputFunc(vw, &msg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(msg, jc.NotNil)

	var wrong string
	err = outputFunc(vw, &wrong)
	c.Check(err, jc.ErrorMatches, `unexpected output type \*string`)
}

func (s *ManifoldSuite) TestOutputNotValueWorker(c *jc.C) {
	var ml corelogger.ModelLogger
	err := outputFunc(&stubWorker{}, &ml)
	c.Check(err, jc.ErrorMatches, "in should be a \\*valueWorker.*")
}

func (s *ManifoldSuite) TestControllerBranchSelected(c *jc.C) {
	ml := &stubLogSink{}
	getter := dt.NewStubResources(map[string]any{
		"is-controller-flag": engine.NewStaticFlagWorker(true),
		"controller-log-sink": []any{
			corelogger.ModelLogger(ml),
			corelogger.LoggerContextGetter(ml),
			corelogger.ModelLogSinkGetter(ml),
		},
	}).Getter()

	manifold := Manifold(ManifoldConfig{
		ControllerFlagName:       "is-controller-flag",
		ControllerLogSinkName:    "controller-log-sink",
		NonControllerLogSinkName: "non-controller-log-sink",
	})

	w, err := manifold.Start(c.Context(), getter)
	c.Assert(err, jc.ErrorIsNil)

	var out corelogger.ModelLogger
	err = outputFunc(w, &out)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(out, jc.Equals, ml)
}

func (s *ManifoldSuite) TestNonControllerBranchSelected(c *jc.C) {
	ml := &stubLogSink{}
	getter := dt.NewStubResources(map[string]any{
		"is-controller-flag": engine.NewStaticFlagWorker(false),
		"non-controller-log-sink": []any{
			corelogger.ModelLogger(ml),
			corelogger.LoggerContextGetter(ml),
			corelogger.ModelLogSinkGetter(ml),
		},
	}).Getter()

	manifold := Manifold(ManifoldConfig{
		ControllerFlagName:       "is-controller-flag",
		ControllerLogSinkName:    "controller-log-sink",
		NonControllerLogSinkName: "non-controller-log-sink",
	})

	w, err := manifold.Start(c.Context(), getter)
	c.Assert(err, jc.ErrorIsNil)

	var out corelogger.ModelLogger
	err = outputFunc(w, &out)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(out, jc.Equals, ml)
}

func (s *ManifoldSuite) TestMissingActiveBranch(c *jc.C) {
	getter := dt.NewStubResources(map[string]any{
		"is-controller-flag":      engine.NewStaticFlagWorker(true),
		"controller-log-sink":     dependency.ErrMissing,
		"non-controller-log-sink": dependency.ErrMissing,
	}).Getter()

	manifold := Manifold(ManifoldConfig{
		ControllerFlagName:       "is-controller-flag",
		ControllerLogSinkName:    "controller-log-sink",
		NonControllerLogSinkName: "non-controller-log-sink",
	})

	_, err := manifold.Start(c.Context(), getter)
	c.Check(errors.Cause(err), jc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingFlag(c *jc.C) {
	getter := dt.NewStubResources(map[string]any{
		"is-controller-flag": dependency.ErrMissing,
	}).Getter()

	manifold := Manifold(ManifoldConfig{
		ControllerFlagName:       "is-controller-flag",
		ControllerLogSinkName:    "controller-log-sink",
		NonControllerLogSinkName: "non-controller-log-sink",
	})

	_, err := manifold.Start(c.Context(), getter)
	c.Check(errors.Cause(err), jc.Equals, dependency.ErrMissing)
}

type stubWorker struct{ worker.Worker }

type stubLogSink struct{}

func (s *stubLogSink) GetLogWriter(_ context.Context, _ model.UUID) (corelogger.LogWriter, error) {
	return &stubLogWriter{}, nil
}

func (s *stubLogSink) GetLoggerContext(_ context.Context, _ model.UUID) (corelogger.LoggerContext, error) {
	return nil, nil
}

type stubLogWriter struct{}

func (s *stubLogWriter) Log(_ []corelogger.LogRecord) error {
	return nil
}
