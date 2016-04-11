// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/worker/dependency"
)

// StubResource is used to define the behaviour of a StubGetResource func for a
// particular resource name.
type StubResource struct {
	Output interface{}
	Error  error
}

// NewStubResources converts raw into a StubResources by assuming that any non-error
// value is intended to be an output.
func NewStubResources(raw map[string]interface{}) StubResources {
	resources := StubResources{}
	for name, value := range raw {
		if err, ok := value.(error); ok {
			resources[name] = StubResource{Error: err}
		} else {
			resources[name] = StubResource{Output: value}
		}
	}
	return resources
}

// StubResources defines the complete behaviour of a StubGetResource func.
type StubResources map[string]StubResource

// Context returns a dependency.Context that never aborts, backed by resources.
func (resources StubResources) Context() dependency.Context {
	return &Context{
		resources: resources,
	}
}

// StubContext returns a Context backed by abort and resources derived from raw.
func StubContext(abort <-chan struct{}, raw map[string]interface{}) *Context {
	return &Context{
		abort:     abort,
		resources: NewStubResources(raw),
	}
}

// Context implements dependency.Context for convenient testing of dependency.StartFuncs.
type Context struct {
	abort     <-chan struct{}
	resources StubResources
}

// Abort is part of the dependency.Context interface.
func (ctx *Context) Abort() <-chan struct{} {
	return ctx.abort
}

// Get is part of the dependency.Context interface.
func (ctx *Context) Get(name string, outPtr interface{}) error {
	resource, found := ctx.resources[name]
	if !found {
		return errors.Errorf("unexpected resource name: %s", name)
	} else if resource.Error != nil {
		return resource.Error
	}
	if outPtr != nil {
		outPtrV := reflect.ValueOf(outPtr)
		if outPtrV.Kind() != reflect.Ptr {
			return errors.Errorf("outPtr should be a pointer; is %#v", outPtr)
		}
		outV := outPtrV.Elem()
		outT := outV.Type()
		setV := reflect.ValueOf(resource.Output)
		setT := setV.Type()
		if !setT.ConvertibleTo(outT) {
			return errors.Errorf("cannot set %#v into %T", resource.Output, outPtr)
		}
		outV.Set(setV.Convert(outT))
	}
	return nil
}
