// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

type basicInitSystem interface {
	Name() string
	Validate(name string, conf Conf) error
	Serialize(name string, conf Conf) ([]byte, error)
	Deserialize(data []byte, name string) (*Conf, error)
}

// Tracking is an InitSystem implementation that doesn't
// actually interact with a concrete init system. Instead it simulates
// one by tracking services (and their state) in memory. For validation
// and serialization it defers to its base InitSystem.
type Tracking struct {
	base basicInitSystem
	fops fileOperations

	// Services is the record of known services and their configurations.
	Services map[string]Conf

	// Enabled tracks which services are currently enabled.
	Enabled set.Strings

	// Running tracks which services are currently running.
	Running set.Strings
}

// NewTracking creates a new Tracking around the provided init system
// and "filesystem".
func NewTracking(base basicInitSystem, fops fileOperations) *Tracking {
	return &Tracking{
		base:     base,
		fops:     fops,
		Services: make(map[string]Conf),
		Enabled:  set.NewStrings(),
		Running:  set.NewStrings(),
	}
}

// Name implements InitSystem.
func (is *Tracking) Name() string {
	return is.base.Name()
}

// List implements InitSystem.
func (is *Tracking) List(include ...string) ([]string, error) {
	if len(include) == 0 {
		return is.Enabled.Values(), nil
	}

	var names []string
	for _, name := range is.Enabled.Values() {
		for _, included := range include {
			if name == included {
				names = append(names, name)
				break
			}
		}
	}
	return names, nil
}

// Start implements InitSystem.
func (is *Tracking) Start(name string) error {
	if err := EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}
	if is.Running.Contains(name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	is.Running.Add(name)
	return nil
}

// Stop implements InitSystem.
func (is *Tracking) Stop(name string) error {
	if err := EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}
	if !is.Running.Contains(name) {
		return errors.NotFoundf("service %q", name)
	}

	is.Running.Remove(name)
	return nil
}

// Enable implements InitSystem.
func (is *Tracking) Enable(name, filename string) error {
	if err := EnsureEnabled(name, is); err == nil {
		return errors.AlreadyExistsf("service %q", name)
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	conf, err := ReadConf(name, filename, is, is.fops)
	if err != nil {
		return errors.Trace(err)
	}

	is.Services[name] = *conf
	is.Enabled.Add(name)
	return nil
}

// Disable implements InitSystem.
func (is *Tracking) Disable(name string) error {
	if err := EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	is.Enabled.Remove(name)
	delete(is.Services, name)
	return nil
}

// IsEnabled implements InitSystem.
func (is *Tracking) IsEnabled(name string) (bool, error) {
	return is.Enabled.Contains(name), nil
}

// Check implements InitSystem.
func (is *Tracking) Check(name, filename string) (bool, error) {
	matched, err := CheckConf(name, filename, is, is.fops)
	return matched, errors.Trace(err)
}

// Info implements InitSystem.
func (is *Tracking) Info(name string) (*ServiceInfo, error) {
	if err := EnsureEnabled(name, is); err != nil {
		return nil, errors.Trace(err)
	}

	status := StatusStopped
	if is.Running.Contains(name) {
		status = StatusRunning
	}

	conf := is.Services[name]
	info := ServiceInfo{
		Name:        name,
		Description: conf.Desc,
		Status:      status,
	}
	return &info, nil
}

// Conf implements InitSystem.
func (is *Tracking) Conf(name string) (*Conf, error) {
	if err := EnsureEnabled(name, is); err != nil {
		return nil, errors.Trace(err)
	}

	conf := is.Services[name]
	return &conf, nil
}

// Validate implements InitSystem.
func (is *Tracking) Validate(name string, conf Conf) error {
	err := is.base.Validate(name, conf)
	return errors.Trace(err)
}

// Serialize implements InitSystem.
func (is *Tracking) Serialize(name string, conf Conf) ([]byte, error) {
	data, err := is.base.Serialize(name, conf)
	return data, errors.Trace(err)
}

// Deserialize implements InitSystem.
func (is *Tracking) Deserialize(data []byte, name string) (*Conf, error) {
	conf, err := is.base.Deserialize(data, name)
	return conf, errors.Trace(err)
}
