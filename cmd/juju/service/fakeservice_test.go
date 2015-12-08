// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// fakeClientAPI is the fake client API for testing the service set,
// get and unset commands.  It implements the following interfaces:
// SetServiceAPI, UnsetServiceAPI and GetServiceAPI
type fakeClientAPI struct {
	values    map[string]interface{}
	servName  string
	charmName string
	err       error
}

func (f *fakeClientAPI) Close() error {
	return nil
}

func (f *fakeClientAPI) ServiceGet(service string) (*params.ServiceGetResults, error) {
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

func (f *fakeClientAPI) ServiceSet(service string, options map[string]string) error {
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

func (f *fakeClientAPI) ServiceUnset(service string, options []string) error {
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

// fakeServiceAPI is the fake service API for testing the service
// update command.
type fakeServiceAPI struct {
	serviceName string
	config      string
	err         error
}

func (f *fakeServiceAPI) ServiceUpdate(args params.ServiceUpdate) error {
	if f.err != nil {
		return f.err
	}

	if args.ServiceName != f.serviceName {
		return errors.NotFoundf("service %q", args.ServiceName)
	}

	f.config = args.SettingsYAML
	return nil
}
