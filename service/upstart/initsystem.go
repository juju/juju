// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/symlink"

	"github.com/juju/juju/service/common"
)

// These are vars rather than consts for the sake of testing.
var (
	// ConfDir holds the default init directory name.
	ConfDir = "/etc/init"
)

var InstallStartRetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 250 * time.Millisecond,
}

type initSystem struct {
	name    string
	initDir string
}

func NewInitSystem(name string) common.InitSystem {
	return &initSystem{
		name:    name,
		initDir: ConfDir,
	}
}

// confPath returns the path to the service's configuration file.
func (is initSystem) confPath(name string) string {
	return path.Join(is.initDir, name+".conf")
}

func (is *initSystem) ensureEnabled(name string) error {
	enabled, err := is.IsEnabled(name)
	if err != nil {
		return errors.Trace(err)
	}
	if !enabled {
		return errors.NotFoundf("service %q", name)
	}
	return nil
}

func (is initSystem) Name() string {
	return is.name
}

var servicesRe = regexp.MustCompile("^([a-zA-Z0-9-_:]+)\\.conf$")

func (is *initSystem) List(include ...string) ([]string, error) {
	// TODO(ericsnow) We should be able to use initctl to do this.
	var services []string
	fis, err := ioutil.ReadDir(is.initDir)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		if groups := servicesRe.FindStringSubmatch(fi.Name()); len(groups) > 0 {
			services = append(services, groups[1])
		}
	}
	return services, nil
}

func (is *initSystem) Start(name string) error {
	if err := is.ensureEnabled(name); err != nil {
		return errors.Trace(err)
	}

	if is.isRunning(name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	// On slower disks, upstart may take a short time to realise
	// that there is a service there.
	var err error
	for attempt := InstallStartRetryAttempts.Start(); attempt.Next(); {
		if err = is.start(name); err == nil {
			break
		}
	}
	return errors.Trace(err)
}

func (is *initSystem) start(name string) error {
	err := runCommand("start", "--system", name)
	if err != nil {
		// Double check to see if we were started before our command ran.
		if is.isRunning(name) {
			return nil
		}
		return errors.Trace(err)
	}
	return nil
}

func (is *initSystem) Stop(name string) error {
	if err := is.ensureEnabled(name); err != nil {
		return errors.Trace(err)
	}

	if !is.isRunning(name) {
		return errors.NotFoundf("service %q", name)
	}

	err := runCommand("stop", "--system", name)
	return errors.Trace(err)
}

func (is *initSystem) Enable(name, filename string) error {
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

func (is *initSystem) Disable(name string) error {
	if err := is.ensureEnabled(name); err != nil {
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

func (is *initSystem) IsEnabled(name string) (bool, error) {
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

func (is *initSystem) Info(name string) (*common.ServiceInfo, error) {
	if err := is.ensureEnabled(name); err != nil {
		return nil, errors.Trace(err)
	}

	status := common.StatusStopped
	if is.isRunning(name) {
		status = common.StatusRunning
	}

	info := &common.ServiceInfo{
		Name:   name,
		Status: status,
	}
	return info, nil
}

var startedRE = regexp.MustCompile(`^.* start/running, process (\d+)\n$`)

func (is *initSystem) isRunning(name string) bool {
	cmd := exec.Command("status", "--system", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// TODO(ericsnow) Are we really okay ignoring the error?
		return false
	}
	return startedRE.Match(out)
}

func (is *initSystem) Conf(name string) (*common.Conf, error) {
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

func (is *initSystem) Serialize(name string, conf common.Conf) ([]byte, error) {
	data, err := Serialize(name, conf)
	return data, errors.Trace(err)
}

func (is *initSystem) Deserialize(data []byte) (*common.Conf, error) {
	conf, err := Deserialize(data)
	return conf, errors.Trace(err)
}
