// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io/ioutil"
	"runtime"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
	"github.com/juju/juju/service/initsystems/upstart"
	"github.com/juju/juju/service/initsystems/windows"
	"github.com/juju/juju/version"
)

// These are the names of the juju-compatible init systems.
const (
	InitSystemWindows = "windows"
	InitSystemUpstart = "upstart"
)

var (
	// initSystemExecutables maps the executable from PID 1 onto the
	// name of an init system.
	initSystemExecutables = map[string]string{
		"<windows>":  InitSystemWindows,
		"/sbin/init": InitSystemUpstart,
	}
)

// newInitSystem returns an InitSystem implementation based on the
// provided name. If the name is unrecognized then errors.NotFound is
// returned.
func newInitSystem(name string) (initsystems.InitSystem, error) {
	switch name {
	case InitSystemWindows:
		return windows.NewInitSystem(name), nil
	case InitSystemUpstart:
		return upstart.NewInitSystem(name), nil
	}
	return nil, errors.NotFoundf("init system implementation for %q", name)
}

// TODO(ericsnow) Support discovering init system on remote host.

// DiscoverInitSystem determines the name of the init system to use
// and returns it. The name is derived from the executable of PID 1.
// If that does work then the information in version.Current is used
// to decide which init system. If the init system cannot be
// discovered at all then the empty string is returned.
func DiscoverInitSystem() string {
	// First try to "read" the name.
	name, err := readLocalInitSystem()
	if err == nil {
		return name
	}

	// Fall back to checking what juju knows about the OS.
	return osInitSystem(version.Current)
}

func readLocalInitSystem() (string, error) {
	executable, err := findInitExecutable()
	if err != nil {
		return "", errors.Annotate(err, "while finding init exe")
	}

	name, ok := initSystemExecutables[executable]
	if !ok {
		return "", errors.NotFoundf("unrecognized init system")
	}

	return name, nil
}

var findInitExecutable = func() (string, error) {
	if runtime.GOOS == "windows" {
		return "<windows>", nil
	}

	// This should work on all linux-like OSes.
	data, err := ioutil.ReadFile("/proc/1/cmdline")
	if err == nil {
		return strings.Fields(string(data))[0], nil
	}

	return "", errors.Trace(err)
}

func osInitSystem(vers version.Binary) string {
	switch vers.OS {
	case version.Windows:
		return InitSystemWindows
	case version.Ubuntu:
		switch vers.Series {
		case "precise", "quantal", "raring", "saucy", "trusty", "utopic":
			return InitSystemUpstart
		default:
			// vivid and later...
			return ""
			//return InitSystemSystemd
		}
	case version.CentOS:
		return ""
		//return InitSystemSystemd
	default:
		return ""
	}
}
