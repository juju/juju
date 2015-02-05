// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"os"
	"path"
	"regexp"

	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
)

// Vars for patching in tests.
var (
	// ConfDir holds the default init directory name.
	ConfDir = "/etc/init"
)

var (
	servicesRe = regexp.MustCompile("^([a-zA-Z0-9-_:]+)\\.conf$")
	startedRE  = regexp.MustCompile(`^.* start/running, process (\d+)\n$`)
)

// Upstart is an InitSystem implementation for upstart.
type Upstart struct {
	name    string
	initDir string
	fops    fileOperations
	cmd     cmdRunner
}

// NewInitSystem returns a new value that implements
// initsystems.InitSystem for upstart.
func NewInitSystem(name string) initsystems.InitSystem {
	return &Upstart{
		name:    name,
		initDir: ConfDir,
		fops:    newFileOperations(),
		cmd:     newCmdRunner(),
	}
}

// confPath returns the path to the service's configuration file.
func (is Upstart) confPath(name string) string {
	return path.Join(is.initDir, name+".conf")
}

// Name implements initsystems.InitSystem.
func (is Upstart) Name() string {
	if is.name == "" {
		return "upstart"
	}
	return is.name
}

// List implements initsystems.InitSystem.
func (is *Upstart) List(include ...string) ([]string, error) {
	// TODO(ericsnow) We should be able to use initctl to do this.
	var services []string
	fis, err := is.fops.ListDir(is.initDir)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		groups := servicesRe.FindStringSubmatch(fi.Name())
		if len(groups) > 0 {
			services = append(services, groups[1])
		}
	}

	return initsystems.FilterNames(services, include), nil
}

// Start implements initsystems.InitSystem.
func (is *Upstart) Start(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	if err := is.ensureRunning(name); err == nil {
		return errors.AlreadyExistsf("service %q", name)
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	// On slower disks, upstart may take a short time to realise
	// that there is a service there.
	var err error
	for attempt := initsystems.RetryAttempts.Start(); attempt.Next(); {
		if err = is.start(name); err == nil {
			break
		}
	}
	return errors.Trace(err)
}

func (is *Upstart) start(name string) error {
	_, err := is.cmd.RunCommand("start", "--system", name)
	if err != nil {
		// Double check to see if we were started before our command ran.
		if err := is.ensureRunning(name); err == nil {
			return nil
		}
		return errors.Trace(err)
	}
	return nil
}

// Stop implements initsystems.InitSystem.
func (is *Upstart) Stop(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	if err := is.ensureRunning(name); err != nil {
		return errors.Trace(err)
	}

	_, err := is.cmd.RunCommand("stop", "--system", name)
	return errors.Trace(err)
}

// Enable implements initsystems.InitSystem.
func (is *Upstart) Enable(name, filename string) error {
	enabled, err := is.IsEnabled(name)
	if err != nil {
		return errors.Trace(err)
	}
	if enabled {
		return errors.AlreadyExistsf("service %q", name)
	}

	// Deserialize and validate the file.
	if _, err := initsystems.ReadConf(name, filename, is, is.fops); err != nil {
		return errors.Trace(err)
	}

	err = is.fops.Symlink(filename, is.confPath(name))
	return errors.Trace(err)
}

// Disable implements initsystems.InitSystem.
func (is *Upstart) Disable(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	return is.fops.RemoveAll(is.confPath(name))
}

// IsEnabled implements initsystems.InitSystem.
func (is *Upstart) IsEnabled(name string) (bool, error) {
	// TODO(ericsnow) In the general case, relying on the conf file
	// may not be the safest route. Perhaps we should use initctl?
	exists, err := is.fops.Exists(is.confPath(name))
	if err != nil {
		return false, errors.Trace(err)
	}
	return exists, nil
}

// Check implements initsystems.InitSystem.
func (is *Upstart) Check(name, filename string) (bool, error) {
	actual, err := is.fops.Readlink(is.confPath(name))
	if err != nil {
		return false, errors.Trace(err)
	}
	return actual == filename, nil
}

// Info implements initsystems.InitSystem.
func (is *Upstart) Info(name string) (*initsystems.ServiceInfo, error) {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return nil, errors.Trace(err)
	}

	conf, err := is.Conf(name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	status := initsystems.StatusStopped
	if err := is.ensureRunning(name); err == nil {
		status = initsystems.StatusRunning
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	info := &initsystems.ServiceInfo{
		Name:        name,
		Description: conf.Desc,
		Status:      status,
	}
	return info, nil
}

func (is *Upstart) ensureRunning(name string) error {
	out, err := is.cmd.RunCommand("status", "--system", name)
	if err != nil {
		return errors.Trace(err)
	}
	if !startedRE.Match(out) {
		return errors.NotFoundf("service %q", name)
	}
	return nil
}

// Conf implements initsystems.InitSystem.
func (is *Upstart) Conf(name string) (*initsystems.Conf, error) {
	data, err := is.fops.ReadFile(is.confPath(name))
	if os.IsNotExist(err) {
		return nil, errors.NotFoundf("service %q", name)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	conf, err := is.Deserialize(data, name)
	return conf, errors.Trace(err)
}

// Validate implements initsystems.InitSystem.
func (is *Upstart) Validate(name string, conf initsystems.Conf) error {
	err := Validate(name, conf)
	return errors.Trace(err)
}

// Serialize implements initsystems.InitSystem.
func (Upstart) Serialize(name string, conf initsystems.Conf) ([]byte, error) {
	data, err := Serialize(name, conf)
	return data, errors.Trace(err)
}

// Deserialize implements initsystems.InitSystem.
func (is *Upstart) Deserialize(data []byte, name string) (*initsystems.Conf, error) {
	conf, err := Deserialize(data, name)
	return conf, errors.Trace(err)
}
