// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// fakeServiceAPI is the fake service API for testing the service
// update command.
type fakeServiceAPI struct {
	serviceName string
	charmName   string
	values      map[string]interface{}
	config      string
	err         error
}

func (f *fakeServiceAPI) Update(args params.ServiceUpdate) error {
	if f.err != nil {
		return f.err
	}

	if args.ServiceName != f.serviceName {
		return errors.NotFoundf("service %q", args.ServiceName)
	}

	f.config = args.SettingsYAML
	return nil
}

func (f *fakeServiceAPI) Close() error {
	return nil
}

func (f *fakeServiceAPI) Get(service string) (*params.ServiceGetResults, error) {
	if service != f.serviceName {
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
		Service: f.serviceName,
		Charm:   f.charmName,
		Config:  configInfo,
	}, nil
}

func (f *fakeServiceAPI) Set(service string, options map[string]string) error {
	if f.err != nil {
		return f.err
	}

	if service != f.serviceName {
		return errors.NotFoundf("service %q", service)
	}

	if f.values == nil {
		f.values = make(map[string]interface{})
	}
	for k, v := range options {
		f.values[k] = v
	}

	return nil
}

func (f *fakeServiceAPI) Unset(service string, options []string) error {
	if f.err != nil {
		return f.err
	}

	if service != f.serviceName {
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
