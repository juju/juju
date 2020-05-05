// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/os"
	"github.com/juju/os/series"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
)

// DiscoverService returns an interface to a service appropriate
// for the current system
func DiscoverService(name string, conf common.Conf) (Service, error) {
	initName, err := discoverInitSystem(series.MustHostSeries())
	if err != nil {
		return nil, errors.Trace(err)
	}

	service, err := newService(name, conf, initName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return service, nil
}

func discoverInitSystem(hostSeries string) (string, error) {
	initName, err := discoverLocalInitSystem()
	if errors.IsNotFound(err) {
		// Fall back to checking the juju version.
		versionInitName, err2 := VersionInitSystem(hostSeries)
		if err2 != nil {
			// The key error is the one from discoverLocalInitSystem so
			// that is what we return.
			return "", errors.Wrap(err2, err)
		}
		initName = versionInitName
	} else if err != nil {
		return "", errors.Trace(err)
	}
	return initName, nil
}

// VersionInitSystem returns an init system name based on the provided
// series. If one cannot be identified a NotFound error is returned.
func VersionInitSystem(series string) (string, error) {
	initName, err := versionInitSystem(series)
	if err != nil {
		return "", errors.Trace(err)
	}
	logger.Debugf("discovered init system %q from series %q", initName, series)
	return initName, nil
}

func versionInitSystem(ser string) (string, error) {
	seriesos, err := series.GetOSFromSeries(ser)
	if err != nil {
		notFound := errors.NotFoundf("init system for series %q", ser)
		return "", errors.Wrap(err, notFound)
	}

	switch seriesos {
	case os.Windows:
		return InitSystemWindows, nil
	case os.Ubuntu:
		switch ser {
		case "precise", "quantal", "raring", "saucy", "trusty", "utopic":
			return InitSystemUpstart, nil
		default:
			// vivid and later
			if featureflag.Enabled(feature.LegacyUpstart) {
				return InitSystemUpstart, nil
			}
			return InitSystemSystemd, nil
		}
	case os.CentOS:
		return InitSystemSystemd, nil
	case os.OpenSUSE:
		return InitSystemSystemd, nil
	}
	return "", errors.NotFoundf("unknown os %q (from series %q), init system", seriesos, ser)
}

type discoveryCheck struct {
	name      string
	isRunning func() (bool, error)
}

var discoveryFuncs = []discoveryCheck{
	{InitSystemUpstart, upstart.IsRunning},
	{InitSystemSystemd, func() (bool, error) { return systemd.IsRunning(), nil }},
	{InitSystemWindows, windows.IsRunning},
}

func discoverLocalInitSystem() (string, error) {
	for _, check := range discoveryFuncs {
		local, err := check.isRunning()
		if err != nil {
			logger.Debugf("failed to find init system %q: %v", check.name, err)
		}
		// We expect that in error cases "local" will be false.
		if local {
			logger.Debugf("discovered init system %q from local host", check.name)
			return check.name, nil
		}
	}
	return "", errors.NotFoundf("init system (based on local host)")
}

const (
	discoverSystemd = "if [ -d /run/systemd/system ]; then echo -n systemd; exit 0; fi"
	discoverUpstart = "if [ -f /sbin/initctl ] && /sbin/initctl --system list 2>&1 > /dev/null; then echo -n upstart; exit 0; fi"
)

// DiscoverInitSystemScript returns the shell script to use when
// discovering the local init system. The script is quite specific to
// bash, so it includes an explicit bash shbang.
func DiscoverInitSystemScript() string {
	renderer := shell.BashRenderer{}
	tests := []string{
		discoverSystemd,
		discoverUpstart,
		"exit 1",
	}
	data := renderer.RenderScript(tests)
	return string(data)
}

// newShellSelectCommand creates a bash case statement with clause for
// each of the linux init systems. The body of each clause comes from
// calling the provided handler with the init system name. If the
// handler does not support the args then it returns a false "ok" value.
func newShellSelectCommand(envVarName, defaultCase string, handler func(string) (string, bool)) string {
	var cases []string

	const shellCaseStatement = `
case "$%s" in
%s
*)
    %s
    ;;
esac`

	for _, initSystem := range linuxInitSystems {
		cmd, ok := handler(initSystem)
		if !ok {
			continue
		}
		cases = append(cases, initSystem+")", "    "+cmd, "    ;;")
	}
	if len(cases) == 0 {
		return ""
	}

	return fmt.Sprintf(shellCaseStatement[1:], envVarName, strings.Join(cases, "\n"), defaultCase)
}
