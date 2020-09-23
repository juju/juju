// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// fakeApplicationAPI is the fake application API for testing the application
// update command.
type fakeApplicationAPI struct {
	branchName  string
	name        string
	charmName   string
	charmValues map[string]interface{}
	appValues   map[string]interface{}
	config      string
	err         error
	version     int
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

func (f *fakeApplicationAPI) BestAPIVersion() int {
	return f.version
}

func (f *fakeApplicationAPI) Close() error {
	return nil
}

func (f *fakeApplicationAPI) Get(branchName, application string) (*params.ApplicationGetResults, error) {
	if branchName != f.branchName {
		return nil, errors.Errorf("expected branch %q, got %q", f.branchName, branchName)
	}

	if application != f.name {
		return nil, errors.NotFoundf("application %q", application)
	}

	charmConfigInfo := make(map[string]interface{})
	for k, v := range f.charmValues {
		charmConfigInfo[k] = map[string]interface{}{
			"description": fmt.Sprintf("Specifies %s", k),
			"type":        fmt.Sprintf("%T", v),
			"value":       v,
		}
	}
	appConfigInfo := make(map[string]interface{})
	for k, v := range f.appValues {
		appConfigInfo[k] = map[string]interface{}{
			"description": fmt.Sprintf("Specifies %s", k),
			"type":        fmt.Sprintf("%T", v),
			"value":       v,
		}
	}

	return &params.ApplicationGetResults{
		Application:       f.name,
		Charm:             f.charmName,
		CharmConfig:       charmConfigInfo,
		ApplicationConfig: appConfigInfo,
	}, nil
}

func (f *fakeApplicationAPI) Set(application string, options map[string]string) error {
	if f.err != nil {
		return f.err
	}

	if application != f.name {
		return errors.NotFoundf("application %q", application)
	}

	charmKeys := set.NewStrings("title", "skill-level", "username", "outlook")
	if f.charmValues == nil {
		f.charmValues = make(map[string]interface{})
	}
	for k, v := range options {
		if charmKeys.Contains(k) {
			f.charmValues[k] = v
		} else {
			f.appValues[k] = v
		}
	}

	return nil
}

func (f *fakeApplicationAPI) SetApplicationConfig(branchName, application string, config map[string]string) error {
	if branchName != f.branchName {
		return errors.Errorf("expected branch %q, got %q", f.branchName, branchName)
	}
	return f.Set(application, config)
}

func (f *fakeApplicationAPI) SetConfig(branchName, application, configYAML string, config map[string]string) error {
	if branchName != f.branchName {
		return errors.Errorf("expected branch %q, got %q", f.branchName, branchName)
	}
	if f.err != nil {
		return f.err
	}

	if application != f.name {
		return errors.NotFoundf("application %q", application)
	}

	f.config = configYAML
	return f.Set(application, config)
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
		if _, ok := f.appValues[name]; ok {
			continue
		}
		if _, ok := f.charmValues[name]; !ok {
			return errors.Errorf("unknown option %q", name)
		}
	}

	for _, name := range options {
		delete(f.charmValues, name)
		delete(f.appValues, name)
	}

	return nil
}

func (f *fakeApplicationAPI) UnsetApplicationConfig(branchName, application string, options []string) error {
	if branchName != f.branchName {
		return errors.Errorf("expected branch %q, got %q", f.branchName, branchName)
	}
	return f.Unset(application, options)
}
