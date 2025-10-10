// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
)

// NewOwnedWorker returns a degenerate worker that exposes the supplied worker
// when passed into OwnedWorkerOutput. The worker will then be owned and managed
// by the returned ownedWorker. If the owned worker is stopped or killed
// then the contained worker will also be stopped or killed.
func NewOwnedWorker(value worker.Worker) (worker.Worker, error) {
	if value == nil {
		return nil, errors.New("NewOwnedWorker expects a worker")
	}
	w := &ownedWorker{
		value: value,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Name: "owned-worker",
		Work: func() error {
			<-w.catacomb.Dying()
			return w.catacomb.ErrDying()
		},
		Init: []worker.Worker{value},
	}); err != nil {
		return nil, err
	}

	return w, nil
}

// OwnedWorkerOutput sets the value wrapped by the supplied ownedWorker into
// the out pointer, if type-compatible, or fails.
func OwnedWorkerOutput(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*ownedWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a *ownedWorker; is %#v", in)
	}
	outV := reflect.ValueOf(out)
	if outV.Kind() != reflect.Ptr {
		return errors.Errorf("out should be a pointer; is %#v", out)
	}
	outValV := outV.Elem()
	outValT := outValV.Type()
	inValV := reflect.ValueOf(inWorker.value)
	inValT := inValV.Type()
	if !inValT.ConvertibleTo(outValT) {
		return errors.Errorf("cannot output into %T", out)
	}
	outValV.Set(inValV.Convert(outValT))
	return nil
}

// ownedWorker implements a degenerate worker wrapping a single value.
type ownedWorker struct {
	catacomb catacomb.Catacomb
	value    interface{}
}

// Kill is part of the worker.Worker interface.
func (v *ownedWorker) Kill() {
	v.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (v *ownedWorker) Wait() error {
	return v.catacomb.Wait()
}

// Value returns the contained value.
func (v *ownedWorker) Value() interface{} {
	return v.value
}

// Report implements the worker.Reporter interface.
func (v *ownedWorker) Report() map[string]interface{} {
	if reporter, ok := v.value.(worker.Reporter); ok {
		return reporter.Report()
	}
	return nil
}
