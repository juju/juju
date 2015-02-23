// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) Remove build constraints from this package. upstart
// should be viable under Windows as long as its fileOperations and
// cmdRunner are targetting linux (e.g. a remote host). Once those two
// interfaces expose their supported target, upstart should validate
// that target and the build constraints should be removed.
// +build linux

package upstart

import (
	"os"
	"path"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/utils/fs"

	"github.com/juju/juju/service/initsystems"
)

const (
	// Name is the name of the init system.
	Name = "upstart"
)

func init() {
	initsystems.Register(Name, initsystems.InitSystemDefinition{
		Name:        Name,
		OSNames:     []string{"!windows"},
		Executables: []string{"/sbin/init"},
		New:         NewInitSystem,
	})
}

// Vars for patching in tests.
var (
	// confDir holds the default init directory name.
	confDir = "/etc/init"
)

var (
	servicesRe = regexp.MustCompile("^([a-zA-Z0-9-_:]+)\\.conf$")
	startedRE  = regexp.MustCompile(`^.* start/running, process (\d+)\n$`)
)

// fileOperations exposes the parts of fs.Operations used by the upstart
// implementation of InitSystem.
type fileOperations interface {
	// Exists implements fs.Operations.
	Exists(name string) (bool, error)

	// ListDir implements fs.Operations.
	ListDir(dirname string) ([]os.FileInfo, error)

	// ReadFile implements fs.Operations.
	ReadFile(filename string) ([]byte, error)

	// Symlink implements fs.Operations.
	Symlink(oldname, newname string) error

	// Readlink implements fs.Operations.
	Readlink(name string) (string, error)

	// RemoveAll implements fs.Operations.
	RemoveAll(name string) error
}

// cmdRunner exposes the parts of initsystems.Shell used by the upstart
// implementation of InitSystem.
type cmdRunner interface {
	// RunCommend implements initsystems.Shell.
	RunCommand(cmd string, args ...string) ([]byte, error)
}

// upstart is an InitSystem implementation for upstart.
type upstart struct {
	name    string
	initDir string
	fops    fileOperations
	cmd     cmdRunner
}

// NewInitSystem returns a new value that implements
// initsystems.InitSystem for upstart.
func NewInitSystem(name string) initsystems.InitSystem {
	return &upstart{
		name:    name,
		initDir: confDir,
		fops:    &fs.Ops{},
		cmd:     &initsystems.LocalShell{},
	}
}

// confPath returns the path to the service's configuration file.
func (is upstart) confPath(name string) string {
	return path.Join(is.initDir, name+".conf")
}

// Name implements initsystems.InitSystem.
func (is upstart) Name() string {
	if is.name == "" {
		return Name
	}
	return is.name
}

// List implements initsystems.InitSystem.
func (is *upstart) List(include ...string) ([]string, error) {
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
func (is *upstart) Start(name string) error {
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

func (is *upstart) start(name string) error {
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
func (is *upstart) Stop(name string) error {
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
func (is *upstart) Enable(name, filename string) error {
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
func (is *upstart) Disable(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	return is.fops.RemoveAll(is.confPath(name))
}

// IsEnabled implements initsystems.InitSystem.
func (is *upstart) IsEnabled(name string) (bool, error) {
	// TODO(ericsnow) In the general case, relying on the conf file
	// may not be the safest route. Perhaps we should use initctl?
	exists, err := is.fops.Exists(is.confPath(name))
	if err != nil {
		return false, errors.Trace(err)
	}
	return exists, nil
}

// Check implements initsystems.InitSystem.
func (is *upstart) Check(name, filename string) (bool, error) {
	actual, err := is.fops.Readlink(is.confPath(name))
	if err != nil {
		return false, errors.Trace(err)
	}
	return actual == filename, nil
}

// Info implements initsystems.InitSystem.
func (is *upstart) Info(name string) (initsystems.ServiceInfo, error) {
	var info initsystems.ServiceInfo

	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return info, errors.Trace(err)
	}

	conf, err := is.Conf(name)
	if err != nil {
		return info, errors.Trace(err)
	}

	status := initsystems.StatusStopped
	if err := is.ensureRunning(name); err == nil {
		status = initsystems.StatusRunning
	} else if !errors.IsNotFound(err) {
		return info, errors.Trace(err)
	}

	info = initsystems.ServiceInfo{
		Name:        name,
		Description: conf.Desc,
		Status:      status,
	}
	return info, nil
}

func (is *upstart) ensureRunning(name string) error {
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
func (is *upstart) Conf(name string) (initsystems.Conf, error) {
	var conf initsystems.Conf

	data, err := is.fops.ReadFile(is.confPath(name))
	if os.IsNotExist(err) {
		return conf, errors.NotFoundf("service %q", name)
	}
	if err != nil {
		return conf, errors.Trace(err)
	}

	conf, err = is.Deserialize(data, name)
	return conf, errors.Trace(err)
}

// Validate implements initsystems.InitSystem.
func (is *upstart) Validate(name string, conf initsystems.Conf) (string, error) {
	confName, err := Validate(name, conf)
	return confName, errors.Trace(err)
}

// Serialize implements initsystems.InitSystem.
func (upstart) Serialize(name string, conf initsystems.Conf) ([]byte, error) {
	data, err := Serialize(name, conf)
	return data, errors.Trace(err)
}

// Deserialize implements initsystems.InitSystem.
func (upstart) Deserialize(data []byte, name string) (initsystems.Conf, error) {
	conf, err := Deserialize(data, name)
	return conf, errors.Trace(err)
}
