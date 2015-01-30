// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/utils/symlink"

	"github.com/juju/juju/service/initsystems"
)

// Vars for patching in tests.
var (
	// ConfDir holds the default init directory name.
	ConfDir = "/etc/init"
)

var (
	upstartServicesRe = regexp.MustCompile("^([a-zA-Z0-9-_:]+)\\.conf$")
	upstartStartedRE  = regexp.MustCompile(`^.* start/running, process (\d+)\n$`)
)

type fileOperations interface {
	exists(name string) (bool, error)
	readDir(dirname string) ([]os.FileInfo, error)
	readFile(filename string) ([]byte, error)
	symlink(oldname, newname string) error
}

type upstart struct {
	name    string
	initDir string
	fops    fileOperations
}

// NewInitSystem returns a new value that implements
// initsystems.InitSystem for upstart.
func NewInitSystem(name string) initsystems.InitSystem {
	return &upstart{
		name:    name,
		initDir: ConfDir,
	}
}

// confPath returns the path to the service's configuration file.
func (is upstart) confPath(name string) string {
	return path.Join(is.initDir, name+".conf")
}

// Name implements service/initsystems.InitSystem.
func (is upstart) Name() string {
	return is.name
}

// List implements service/initsystems.InitSystem.
func (is *upstart) List(include ...string) ([]string, error) {
	// TODO(ericsnow) We should be able to use initctl to do this.
	var services []string
	fis, err := ioutil.ReadDir(is.initDir)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		groups := upstartServicesRe.FindStringSubmatch(fi.Name())
		if len(groups) > 0 {
			services = append(services, groups[1])
		}
	}
	return services, nil
}

// Start implements service/initsystems.InitSystem.
func (is *upstart) Start(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	if is.isRunning(name) {
		return errors.AlreadyExistsf("service %q", name)
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
	err := initsystems.RunCommand("start", "--system", name)
	if err != nil {
		// Double check to see if we were started before our command ran.
		if is.isRunning(name) {
			return nil
		}
		return errors.Trace(err)
	}
	return nil
}

// Stop implements service/initsystems.InitSystem.
func (is *upstart) Stop(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	if !is.isRunning(name) {
		return errors.NotFoundf("service %q", name)
	}

	err := initsystems.RunCommand("stop", "--system", name)
	return errors.Trace(err)
}

// Enable implements service/initsystems.InitSystem.
func (is *upstart) Enable(name, filename string) error {
	// TODO(ericsnow) Deserialize and validate?

	enabled, err := is.IsEnabled(name)
	if err != nil {
		return errors.Trace(err)
	}
	if enabled {
		return errors.AlreadyExistsf("service %q", name)
	}

	// TODO(ericsnow) Will the symlink have the right permissions?
	err = symlink.New(filename, is.confPath(name))
	return errors.Trace(err)
}

// Disable implements service/initsystems.InitSystem.
func (is *upstart) Disable(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	enabled, err := is.IsEnabled(name)
	if err != nil {
		return errors.Trace(err)
	}
	if enabled {
		return nil
	}

	return os.Remove(is.confPath(name))
}

// IsEnabled implements service/initsystems.InitSystem.
func (is *upstart) IsEnabled(name string) (bool, error) {
	// TODO(ericsnow) In the general case, relying on the conf file
	// may not be the safest route. Perhaps we should use initctl?
	_, err := os.Stat(is.confPath(name))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

// Info implements service/initsystems.InitSystem.
func (is *upstart) Info(name string) (*initsystems.ServiceInfo, error) {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return nil, errors.Trace(err)
	}

	status := initsystems.StatusStopped
	if is.isRunning(name) {
		status = initsystems.StatusRunning
	}

	info := &initsystems.ServiceInfo{
		Name:   name,
		Status: status,
	}
	return info, nil
}

func (is *upstart) isRunning(name string) bool {
	cmd := exec.Command("status", "--system", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// TODO(ericsnow) Are we really okay ignoring the error?
		return false
	}
	return upstartStartedRE.Match(out)
}

// Conf implements service/initsystems.InitSystem.
func (is *upstart) Conf(name string) (*initsystems.Conf, error) {
	data, err := ioutil.ReadFile(is.confPath(name))
	if os.IsNotExist(err) {
		return nil, errors.NotFoundf("service %q", name)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	conf, err := is.Deserialize(data)
	return conf, errors.Trace(err)
}

// Validate implements service/initsystems.InitSystem.
func (is *upstart) Validate(name string, conf initsystems.Conf) error {
	err := Validate(name, conf)
	return errors.Trace(err)
}

// Serialize implements service/initsystems.InitSystem.
func (upstart) Serialize(name string, conf initsystems.Conf) ([]byte, error) {
	data, err := Serialize(name, conf)
	return data, errors.Trace(err)
}

// Deserialize implements service/initsystems.InitSystem.
func (is *upstart) Deserialize(data []byte) (*initsystems.Conf, error) {
	conf, err := Deserialize(data)
	return conf, errors.Trace(err)
}
