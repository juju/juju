// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io/ioutil"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/version"
)

// These are the names of the juju-compatible init systems.
const (
	InitSystemWindows = "windows"
	InitSystemUpstart = "upstart"
	//InitSystemSystemd = "systemd"
)

var (
	linuxInitNames = map[string]string{
		"/sbin/init": initSystemUpstart,
		//"/sbin/systemd": initSystemSystemd,
	}
	initSystems = map[string](func() common.InitSystem){
		InitSystemWindows: windows.NewInitSystem,
		InitSystemUpstart: upstart.NewInitSystem,
		//InitSystemSystemd: systemd.NewInitSystem,
	}
)

// discoverInitSystem determines which init system is running and
// returns its name.
func discoverInitSystem() (string, error) {
	if version.Current.OS == version.Windows {
		return initSystemWindows, nil
	}

	executable, err := findInitExecutable()
	if err != nil {
		return "", errors.Annotate(err, "while finding init exe")
	}

	name, ok := linuxInitNames[executable]
	if !ok {
		return "", errors.New("unrecognized init system")
	}

	return name, nil
}

var findInitExecutable = func() (string, error) {
	data, err := ioutil.ReadFile("/proc/1/cmdline")
	if err != nil {
		return "", errors.Trace(err)
	}
	return strings.Fields(string(data))[0], nil
}
