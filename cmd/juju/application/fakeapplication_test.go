// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// fakeServiceAPI is the fake application API for testing the application
// update command.
type fakeApplicationAPI struct {
	name      string
	charmName string
	values    map[string]interface{}
	config    string
	err       error
}

func (f *fakeApplicationAPI) Update(args params.ApplicationUpdate) error {
	if f.err != nil {
		return f.err
	}

	if args.ApplicationName != f.name {
		return errors.NotFoundf("application %q", args.ApplicationName)
	}

	f.config = args.SettingsYAML
	return nil
}

func (f *fakeApplicationAPI) Close() error {
	return nil
}

func (f *fakeApplicationAPI) Get(application string) (*params.ApplicationGetResults, error) {
	if application != f.name {
		return nil, errors.NotFoundf("application %q", application)
	}

	configInfo := make(map[string]interface{})
	for k, v := range f.values {
		configInfo[k] = map[string]interface{}{
			"description": fmt.Sprintf("Specifies %s", k),
			"type":        fmt.Sprintf("%T", v),
			"value":       v,
		}
	}

	return &params.ApplicationGetResults{
		Application: f.name,
		Charm:       f.charmName,
		Config:      configInfo,
	}, nil
}

func (f *fakeApplicationAPI) Set(application string, options map[string]string) error {
	if f.err != nil {
		return f.err
	}

	if application != f.name {
		return errors.NotFoundf("application %q", application)
	}

	if f.values == nil {
		f.values = make(map[string]interface{})
	}
	for k, v := range options {
		f.values[k] = v
	}

	return nil
}

func (f *fakeApplicationAPI) Unset(application string, options []string) error {
	if f.err != nil {
		return f.err
	}

	if application != f.name {
		return errors.NotFoundf("application %q", application)
	}

	// Verify all options before unsetting any of them.
	for _, name := range options {
		if _, ok := f.values[name]; !ok {
			return errors.Errorf("unknown option %q", name)
		}
	}

	for _, name := range options {
		delete(f.values, name)
	}

	return nil
}
