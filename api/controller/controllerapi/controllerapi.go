package controllerapi

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

type Logger interface {
	Debugf(string, ...interface{})
}

type ManifoldConfig struct {
	StateName string
	Logger    Logger
}

func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	return nil
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		state:  statePool.SystemState(),
		logger: config.Logger,
	}
	config.Logger.Debugf("controllerapi worker initialized")
	return common.NewCleanupWorker(w, func() {
		_ = stTracker.Done()
	}), nil
}

func (config ManifoldConfig) output(in worker.Worker, out interface{}) error {
	inWorker, ok := in.(*Worker)
	if !ok {
		return errors.Errorf("expected *Worker, got %T", in)
	}
	switch result := out.(type) {
	case **Worker:
		*result = inWorker
	default:
		return errors.Errorf("expected **Worker, got %T", out)
	}
	return nil
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
		},
		Start:  config.start,
		Output: config.output,
	}
}

type State interface {
	FindEntity(tag names.Tag) (state.Entity, error)
}

type Worker struct {
	state  State
	logger Logger
}

func (w *Worker) Life(tag names.Tag) (v life.Value, err error) {
	w.logger.Debugf("controllerapi.Worker.Life start")
	defer func() {
		w.logger.Debugf("controllerapi.Worker.Life finish v=%v, err=%v", v, err)
	}()
	entity, err := w.state.FindEntity(tag)
	if err != nil {
		return "", err
	}
	lifer, ok := entity.(state.Lifer)
	if !ok {
		return "", errors.NotSupportedf("entity %q does not support life cycles", tag)
	}
	return life.Value(lifer.Life().String()), nil
}

func (w *Worker) Kill() {} // TODO

func (w *Worker) Wait() error { return nil } // TODO
