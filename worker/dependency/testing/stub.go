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

// StubResources defines the complete behaviour of a StubGetResource func.
type StubResources map[string]StubResource

// StubGetResource returns a GetResourceFunc which will set outputs, or
// return errors, as defined by the supplied StubResources. Any unexpected
// usage of the result will return Errorf errors describing the problem; in
// particular, missing resources will not trigger dependency.ErrMissing unless
// specifically configured to do so.
func StubGetResource(resources StubResources) dependency.GetResourceFunc {
	return func(name string, outPtr interface{}) error {
		resource, found := resources[name]
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
}
