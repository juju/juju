// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
)

// These are the directives that may be passed to Services.List.
const (
	DirectiveRunning  = "running"
	DirectiveNoVerify = "noverify"
)

const (
	initDir = "init"
)

var (
	prefixes = []string{
		"juju-",
		"jujud-",
	}

	// ErrNotManaged is returned from Services methods when a named
	// service is not managed by juju.
	ErrNotManaged = errors.New("actual service is not managed by juju")
)

// Services exposes the high-level functionality of an underlying init
// system, relative to juju.
type Services struct {
	services
}

// NewServices populates a new Services and returns it. This includes
// determining which init system is in use on the current host. The
// provided data dir is used as the parent of the directory in which all
// juju-managed service configurations are stored. The names of the
// services located there are extracted and cached. A service conf must
// be there already or be added via the Add method before Services will
// recognize it as juju-managed.
func NewServices(dataDir string) (*Services, error) {
	// Get the underlying init system.
	name, err := discoverInitSystem()
	if err != nil {
		return nil, errors.Trace(err)
	}
	newInitSystem := initSystems[name]
	init := newInitSystem()

	// Build the Services.
	services := Services{services{
		baseDir:  filepath.Join(dataDir, initDir),
		initname: name,
		init:     init,
	}}

	// Ensure that the list of known services is cached.
	err = services.refresh()
	return &services, errors.Trace(err)
}

// List collects the names of all juju-managed services and returns it.
// Directives may be passed to modify the behavior (e.g. filter the list
// down).
func (s Services) List(directives ...string) ([]string, error) {
	runningOnly := false
	noVerify := false
	for _, directive := range directives {
		switch directive {
		case DirectiveRunning:
			runningOnly = true
		case DirectiveNoVerify:
			noVerify = true
		default:
			return nil, errors.NotFoundf("directive %q", directive)
		}
	}

	// Select only desired names.
	var names []string
	if runningOnly {
		running, err := s.init.List(s.names...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !noVerify {
			running, err = s.filterActual(running)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		names = running
	} else {
		names = s.names
	}

	return names, nil
}

// Start starts the named juju-managed service (if enabled).
func (s Services) Start(name string) error {
	if err := s.ensure(name); err != nil {
		return errors.Annotatef(err, "service %q", name)
	}

	err := s.init.Start(name)
	if errors.IsNotFound(err) {
		return errors.Errorf("service %q not enabled", name)
	}
	if errors.IsAlreadyExists(err) {
		// It is already started.
		return nil
	}
	return errors.Trace(err)
}

// Stop stops the named juju-managed service. If it isn't running or
// isn't enabled then nothing happens.
func (s Services) Stop(name string) error {
	if err := s.ensure(name); err != nil {
		return errors.Annotatef(err, "service %q", name)
	}

	err := s.init.Stop(name)
	if errors.IsNotFound(err) {
		// Either it is already stopped or it isn't enabled.
		return nil
	}
	return errors.Trace(err)
}

// IsRunning determines whether or not the named service is running.
func (s Services) IsRunning(name string) (bool, error) {
	if err := s.ensure(name); err != nil {
		return false, errors.Annotatef(err, "service %q", name)
	}
	return s.isRunning(name)
}

func (s Services) isRunning(name string) (bool, error) {
	info, err := s.init.Info(name)
	if errors.IsNotFound(err) {
		// Not enabled.
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return (info.Status == common.StatusRunning), nil
}

// Enable adds the named service to the underlying init system.
func (s Services) Enable(name string) error {
	if err := s.ensure(name); err != nil {
		return errors.Annotatef(err, "service %q", name)
	}

	confdir := s.confDir(name)
	err := s.init.Enable(name, confdir.filename())
	if errors.IsAlreadyExists(err) {
		// It is already enabled.
		return nil
	}
	return errors.Trace(err)
}

// Disable removes the named service from the underlying init system.
func (s Services) Disable(name string) error {
	if err := s.ensure(name); err != nil {
		return errors.Annotatef(err, "service %q", name)
	}

	// TODO(ericsnow) Require that the service be stopped already?
	err := s.disable(name)
	return errors.Trace(err)
}

func (s Services) disable(name string) error {
	err := s.init.Disable(name)
	if errors.IsNotFound(err) {
		// It already wasn't enabled.
		// TODO(ericsnow) Is this correct?
		return nil
	}
	return errors.Trace(err)
}

// StopAndDisable is a helper that simply calls Stop and Disable for the
// named service.
func (s Services) StopAndDisable(name string) error {
	if err := s.Stop(name); err != nil {
		return errors.Trace(err)
	}
	err := s.disable(name)
	return errors.Trace(err)
}

// IsEnabled determines whether or not the named service has been
// added to the underlying init system.
func (s Services) IsEnabled(name string) (bool, error) {
	if err := s.ensure(name); err != nil {
		return false, errors.Annotatef(err, "service %q", name)
	}
	return s.isEnabled(name)
}

// Conf extracts the service Conf for the named service. This is useful
// when comparing an existing service against a proposed one.
func (s Services) Conf(name string) (*common.Conf, error) {
	if err := s.ensure(name); err != nil {
		return nil, errors.Annotatef(err, "service %q", name)
	}

	conf, err := s.init.Conf(name)
	return conf, errors.Trace(err)
}

// Add adds the named service to the directory of juju-related
// service configurations. The provided Conf is used to generate the
// conf file and possibly a script file. Adding a service triggers a
// refresh of the cache of juju-managed service names.
func (s Services) Add(name string, conf *common.Conf) error {
	confdir := s.confDir(name)
	if err := confdir.validate(); err != nil {
		return errors.Trace(err)
	}

	if err := confdir.write(conf, s.init); err != nil {
		return errors.Trace(err)
	}

	// Update the list of juju-managed services.
	err := s.refresh()
	return errors.Trace(err)
}

// Remove removes the conf for the named service from the directory of
// juju-related service configurations. If the service is running or
// otherwise enabled then it is stopped and disabled before the
// removal takes place. Removing a service triggers a refresh of the
// cache of juju-managed service names.
func (s Services) Remove(name string) error {
	if err := s.ensure(name); err != nil {
		return errors.Annotatef(err, "service %q", name)
	}

	// Stop and disable first, if necessary.
	enabled, err := s.isEnabled(name)
	if err != nil {
		return errors.Trace(err)
	}
	if enabled {
		if err := s.StopAndDisable(name); err != nil {
			return errors.Trace(err)
		}
	}

	// Delete the service conf directory.
	confdir := s.confDir(name)
	if err := confdir.remove(); err != nil {
		return errors.Trace(err)
	}

	// Update the list of juju-managed services.
	err = s.refresh()
	return errors.Trace(err)
}

type services struct {
	baseDir    string
	initname   string
	init       common.InitSystem
	skipEnsure bool

	names []string
}

func (s services) refresh() error {
	// TODO(ericsnow) Support filtering this list down?
	names, err := s.list()
	if err != nil {
		return errors.Trace(err)
	}

	s.names = names
	return nil
}

func (s services) isknown(name string) bool {
	for _, known := range s.names {
		if name == known {
			return true
		}
	}
	return false
}

func (s services) list() ([]string, error) {
	dirnames, err := listSubdirectories(s.baseDir)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var names []string
	for _, name := range dirnames {
		if !hasPrefix(name, prefixes...) {
			continue
		}
		if err := s.confDir(name).validate(); err == nil {
			names = append(names, name)
		}
	}
	return names, nil
}

func (s services) confDir(name string) *confDir {
	return &confDir{
		dirname:    filepath.Join(s.baseDir, name),
		initSystem: s.initname,
	}
}

func (s services) confFile(name string) string {
	confname := fmt.Sprintf(filenameConf, s.initname)
	return filepath.Join(s.baseDir, name, confname)
}

func (s Services) ensure(name string) error {
	if !s.isknown(name) {
		return errors.NotFoundf("service %q", name)
	}

	if s.skipEnsure {
		return nil
	}

	matched, err := s.isEnabled(name)
	if err != nil {
		return errors.Trace(err)
	}
	if !matched {
		return errors.Trace(ErrNotManaged)
	}
	return nil
}

func (s services) filterActual(names []string) ([]string, error) {
	var filtered []string
	for _, name := range names {
		matched, err := s.isEnabled(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if matched {
			filtered = append(filtered, name)
		}
	}
	return filtered, nil
}

func (s services) isEnabled(name string) (bool, error) {
	filename := s.confFile(name)
	enabled, err := s.init.IsEnabled(name, filename)
	return enabled, errors.Trace(err)
}
