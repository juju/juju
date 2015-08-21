// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"reflect"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
)

// NewValueWorker returns a degenerate worker that exposes the supplied value
// when passed into ValueWorkerOutput. Please do not supply values that have
// their own dependency or lifecycle considerations; such values will subvert
// the operation of any containing dependency.Engine by insulating it from the
// failures and dependency changes of the contained value.
func NewValueWorker(value interface{}) (worker.Worker, error) {
	if value == nil {
		return nil, errors.New("NewValueWorker expects a value")
	}
	w := &valueWorker{
		value: value,
	}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w, nil
}

// ValueWorkerOutput sets the value wrapped by the supplied valueWorker into
// the out pointer, if type-compatible, or fails.
func ValueWorkerOutput(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*valueWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a *valueWorker; is %#v", in)
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

// valueWorker implements a degenerate worker wrapping a single value.
type valueWorker struct {
	tomb  tomb.Tomb
	value interface{}
}

// Kill is part of the worker.Worker interface.
func (v *valueWorker) Kill() {
	v.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (v *valueWorker) Wait() error {
	return v.tomb.Wait()
}
