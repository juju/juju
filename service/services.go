// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/service/common"
)

// These are the directives that may be passed to Services.List.
const (
	DirectiveRunning  = "running"
	DirectiveNoVerify = "noverify"
)

var (
	jujuPrefixes = []string{
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
	configs *serviceConfigs
	init    InitSystem
}

// DiscoverServices populates a new Services and returns it. This
// includes determining which init system is in use on the current host.
// The provided data dir is used as the parent of the directory in which
// all juju-managed service configurations are stored. The names of the
// services located there are extracted and cached. A service conf must
// be there already or be added via the Manage method before Services
// will recognize it as juju-managed.
func DiscoverServices(dataDir string, args ...string) (*Services, error) {
	if len(args) > 1 {
		return nil, errors.Errorf("at most 1 arg expected, got %d", len(args))
	}

	// Get the init system.
	init, err := extractInitSystem(args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Build the Services.
	services := NewServices(dataDir, init)

	// Ensure that the list of known services is cached.
	err = services.configs.refresh()
	return services, errors.Trace(err)
}

// NewServices build a Services from the provided data dir and init
// system and returns it.
func NewServices(dataDir string, init InitSystem) *Services {
	return &Services{
		configs: newConfigs(dataDir, init.Name(), jujuPrefixes...),
		init:    init,
	}
}

func extractInitSystem(args []string) (InitSystem, error) {
	// Get the init system name from the args.
	var name string
	if len(args) != 0 {
		name = args[0]
	}

	// Fall back to discovery.
	if name == "" {
		discovered, err := discoverInitSystem()
		if err != nil {
			return nil, errors.Trace(err)
		}
		name = discovered
	}

	// Return the corresponding init system.
	return newInitSystem(name), nil
}

// InitSystem identifies which init system is in use.
func (s Services) InitSystem() string {
	return s.init.Name()
}

// List collects the names of all juju-managed services and returns it.
func (s Services) List() ([]string, error) {
	return s.configs.names, nil
}

// List running collects the names of all juju-managed services that are
// current running and returns it.
func (s Services) ListEnabled() ([]string, error) {
	enabledList, err := s.init.List(s.configs.names...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var names []string
	for _, managed := range s.configs.names {
		confDir := s.configs.lookup(managed)
		if confDir == nil {
			continue
		}

		var name string
		for _, enabled := range enabledList {
			if enabled == managed {
				name = managed
				break
			}
		}
		if name == "" {
			continue
		}

		// Make sure it is the juju-managed service.
		same, err := s.compareConf(name, confDir)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !same {
			// A service with the same name is enabled.
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// Start starts the named juju-managed service (if enabled).
func (s Services) Start(name string) error {
	if err := s.ensureManaged(name); err != nil {
		return errors.Trace(err)
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
	if err := s.ensureManaged(name); err != nil {
		return errors.Trace(err)
	}
	err := s.stop(name)
	return errors.Trace(err)
}

func (s Services) stop(name string) error {
	err := s.init.Stop(name)
	if errors.IsNotFound(err) {
		// Either it is already stopped or it isn't enabled.
		return nil
	}
	return errors.Trace(err)
}

// IsRunning determines whether or not the named service is running.
func (s Services) IsRunning(name string) (bool, error) {
	err := s.ensureManaged(name)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}

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
	confDir := s.configs.lookup(name)
	if confDir == nil {
		return errors.NotFoundf("service %q", name)
	}

	err := s.init.Enable(name, confDir.filename())
	if errors.IsAlreadyExists(err) {
		// It is already enabled. Make sure the enabled one is
		// managed by juju.
		same, err := s.compareConf(name, confDir)
		if err != nil {
			return errors.Trace(err)
		}
		if !same {
			return errors.Annotatef(ErrNotManaged, "service %q", name)
		}
		return nil
	}
	return errors.Trace(err)
}

// Disable removes the named service from the underlying init system.
func (s Services) Disable(name string) error {
	if err := s.ensureManaged(name); err != nil {
		return errors.Trace(err)
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

// IsEnabled determines whether or not the named service has been
// added to the underlying init system. If a different service
// (determined by comparing confs) with the same name is enabled then
// errors.AlreadyExists is returned.
func (s Services) IsEnabled(name string) (bool, error) {
	err := s.ensureManaged(name)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}

	enabled, err := s.init.IsEnabled(name)
	return enabled, errors.Trace(err)
}

// Manage adds the named service to the directory of juju-related
// service configurations. The provided Conf is used to generate the
// conf file and possibly a script file.
func (s Services) Manage(name string, conf Conf) error {
	err := s.configs.add(name, conf, s.init)
	return errors.Trace(err)
}

// Remove removes the conf for the named service from the directory of
// juju-related service configurations. If the service is running or
// otherwise enabled then it is stopped and disabled before the
// removal takes place. If the service is not managed by juju then
// nothing happens.
func (s Services) Remove(name string) error {
	confDir := s.configs.lookup(name)
	if confDir == nil {
		return nil
	}
	enabled, err := s.init.IsEnabled(name)
	if err != nil {
		return errors.Trace(err)
	}
	if enabled {
		// We must do this before removing the conf directory.
		same, err := s.compareConf(name, confDir)
		if err != nil {
			return errors.Trace(err)
		}
		enabled = same
	}

	// Remove the managed service config.
	if err := s.configs.remove(name); err != nil {
		return errors.Trace(err)
	}

	// Stop and disable the service, if necessary.
	if enabled {
		if err := s.stop(name); err != nil {
			return errors.Trace(err)
		}
		if err := s.disable(name); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// Install prepares the service, enables it, and starts it.
func (s Services) Install(name string, conf Conf) error {
	if err := s.Manage(name, conf); err != nil {
		return errors.Trace(err)
	}
	if err := s.Enable(name); err != nil {
		return errors.Trace(err)
	}
	if err := s.Start(name); err != nil {
		return errors.Trace(err)
	}
	//return errors.Errorf("%+v", s.configs.names)
	return nil
}

// Check verifies the managed conf for the named service to ensure
// it matches the provided Conf.
func (s Services) Check(name string, conf Conf) (bool, error) {
	// TODO(ericsnow) Finish this.
	return false, nil
}

// IsManaged determines whether or not the named service is
// managed by juju.
func (s Services) IsManaged(name string) bool {
	return s.configs.lookup(name) != nil
}

func (s *Services) NewService(name string, conf Conf) *Service {
	return WrapService(name, conf, s)
}

func (s Services) NewAgentService(tag names.Tag, paths AgentPaths, env map[string]string) (*Service, error) {
	svc, err := WrapAgentService(tag, paths, env, s)
	return svc, errors.Trace(err)
}

func (s Services) ensureManaged(name string) error {
	confDir := s.configs.lookup(name)
	if confDir == nil {
		return errors.NotFoundf("service %q", name)
	}

	enabled, err := s.init.IsEnabled(name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	if !enabled {
		return nil
	}

	// Make sure that the juju-managed conf matches the enabled one.
	same, err := s.compareConf(name, confDir)
	if errors.IsNotSupported(err) {
		// We'll just have to trust.
		return nil
	}
	if !same {
		msg := "managed conf for service %q does not match existing service"
		return errors.Annotatef(ErrNotManaged, msg, name)
	}

	return nil
}

func (s Services) compareConf(name string, confDir *confDir) (bool, error) {
	conf, err := s.init.Conf(name)
	if err != nil {
		return false, errors.Trace(err)
	}

	data, err := confDir.conf()
	if err != nil {
		return false, errors.Trace(err)
	}
	expected, err := s.init.Deserialize(data)
	if err != nil {
		return false, errors.Trace(err)
	}

	//return (*conf == *expected), nil
	return conf.Equals(*expected), nil
}
