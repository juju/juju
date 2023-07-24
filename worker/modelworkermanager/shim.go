// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	stdcontext "context"

	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/state"
)

// StatePoolController implements Controller in terms of a *state.StatePool.
type StatePoolController struct {
	*state.StatePool
	SysLogger         corelogger.Logger
	CtrlConfigService ControllerConfigService
}

// Model is part of the Controller interface.
func (g StatePoolController) Model(modelUUID string) (Model, func(), error) {
	model, ph, err := g.StatePool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return model, func() { ph.Release() }, nil
}

// RecordLogger returns a database logger for the specified model.
func (g StatePoolController) RecordLogger(modelUUID string) (RecordLogger, error) {
	ps, err := g.StatePool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer ps.Release()

	model, err := ps.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	config, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	loggingOutputs, _ := config.LoggingOutput()
	return g.getLoggers(loggingOutputs, ps), nil
}

// Config is part of the Controller interface.
func (g StatePoolController) Config() (controller.Config, error) {
	return g.CtrlConfigService.ControllerConfig(stdcontext.Background())
}

func (g StatePoolController) getLoggers(loggingOutputs []string, st state.ModelSessioner) corelogger.LoggerCloser {
	// If the logging output is empty, then send it to state.
	if len(loggingOutputs) == 0 {
		return state.NewDbLogger(st)
	}

	return corelogger.MakeLoggers(loggingOutputs, corelogger.LoggersConfig{
		SysLogger: func() corelogger.Logger {
			return g.SysLogger
		},
		DBLogger: func() corelogger.Logger {
			return state.NewDbLogger(st)
		},
	})
}
