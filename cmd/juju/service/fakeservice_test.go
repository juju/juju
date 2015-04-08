// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// fakeServiceAPI is the fake client API for testing the service set,
// get and unset commands.  It implements the following interfaces:
// SetServiceAPI, UnsetServiceAPI and GetServiceAPI
type fakeServiceAPI struct {
	values    map[string]interface{}
	servName  string
	charmName string
	config    string
	err       error
}

func (f *fakeServiceAPI) Close() error {
	return nil
}

func (f *fakeServiceAPI) ServiceSetYAML(service string, yaml string) error {
	if f.err != nil {
		return f.err
	}

	if service != f.servName {
		return errors.NotFoundf("service %q", service)
	}

	f.config = yaml
	return nil
}

func (f *fakeServiceAPI) ServiceGet(service string) (*params.ServiceGetResults, error) {
	if service != f.servName {
		return nil, errors.NotFoundf("service %q", service)
	}

	configInfo := make(map[string]interface{})
	for k, v := range f.values {
		configInfo[k] = map[string]interface{}{
			"description": fmt.Sprintf("Specifies %s", k),
			"type":        fmt.Sprintf("%T", v),
			"value":       v,
		}
	}

	return &params.ServiceGetResults{
		Service: f.servName,
		Charm:   f.charmName,
		Config:  configInfo,
	}, nil
}

func (f *fakeServiceAPI) ServiceSet(service string, options map[string]string) error {
	if f.err != nil {
		return f.err
	}

	if service != f.servName {
		return errors.NotFoundf("service %q", service)
	}

	for k, v := range options {
		f.values[k] = v
	}

	return nil
}

func (f *fakeServiceAPI) ServiceUnset(service string, options []string) error {
	if f.err != nil {
		return f.err
	}

	if service != f.servName {
		return errors.NotFoundf("service %q", service)
	}

	// Verify all options before unsetting any of them.
	for _, name := range options {
		if _, ok := f.values[name]; !ok {
			return fmt.Errorf("unknown option %q", name)
		}
	}

	for _, name := range options {
		delete(f.values, name)
	}

	return nil
}
