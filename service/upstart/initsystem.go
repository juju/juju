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

	"github.com/juju/juju/service/common"
)

const (
	// confDir holds the default init directory name.
	confDir = "/etc/init"
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
		initDir: confDir,
	}
}

// confPath returns the path to the service's configuration file.
func (is initSystem) confPath(name string) string {
	return path.Join(is.initDir, name+".conf")
}

func (is initSystem) Name() string {
	return is.name
}

var servicesRe = regexp.MustCompile("^([a-zA-Z0-9-_:]+)\\.conf$")

func (is *initSystem) List(include ...string) ([]string, error) {
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
	// TODO(ericsnow) Finish!
	return nil
}

func (is *initSystem) Stop(name string) error {
	// TODO(ericsnow) Finish!
	return nil
}

func (is *initSystem) Enable(name, filename string) error {
	// TODO(ericsnow) Finish!
	return nil
}

func (is *initSystem) Disable(name string) error {
	// TODO(ericsnow) Finish!
	return nil
}

func (is *initSystem) IsEnabled(name string) (bool, error) {
	// TODO(ericsnow) Finish!
	return false, nil

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
	// TODO(ericsnow) Finish!
	return nil, nil

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
		return false
	}
	return startedRE.Match(out)
}

func (is *initSystem) Conf(name string) (*common.Conf, error) {
	data, err := ioutil.ReadFile(is.confPath(name))
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
