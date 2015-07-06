package service

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/version"
)

// This exists to allow patching during tests.
var getVersion = func() version.Binary {
	return version.Current
}

// DiscoverService returns an interface to a service appropriate
// for the current system
func DiscoverService(name string, conf common.Conf) (Service, error) {
	initName, err := discoverInitSystem()
	if err != nil {
		return nil, errors.Trace(err)
	}

	jujuVersion := getVersion()
	service, err := newService(name, conf, initName, jujuVersion.Series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return service, nil
}

func discoverInitSystem() (string, error) {
	initName, err := discoverLocalInitSystem()
	if errors.IsNotFound(err) {
		// Fall back to checking the juju version.
		jujuVersion := getVersion()
		versionInitName, err2 := VersionInitSystem(jujuVersion.Series)
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

func versionInitSystem(series string) (string, error) {
	os, err := version.GetOSFromSeries(series)
	if err != nil {
		notFound := errors.NotFoundf("init system for series %q", series)
		return "", errors.Wrap(err, notFound)
	}

	switch os {
	case version.Windows:
		return InitSystemWindows, nil
	case version.Ubuntu:
		switch series {
		case "precise", "quantal", "raring", "saucy", "trusty", "utopic":
			return InitSystemUpstart, nil
		default:
			// vivid and later
			if featureflag.Enabled(feature.LegacyUpstart) {
				return InitSystemUpstart, nil
			}
			return InitSystemSystemd, nil
		}
	case version.CentOS:
		return InitSystemSystemd, nil
	}
	return "", errors.NotFoundf("unknown os %q (from series %q), init system", os, series)
}

type discoveryCheck struct {
	name      string
	isRunning func() (bool, error)
}

var discoveryFuncs = []discoveryCheck{
	{InitSystemUpstart, upstart.IsRunning},
	{InitSystemSystemd, systemd.IsRunning},
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

const discoverInitSystemScript = `
# Use guaranteed discovery mechanisms for known init systems.
if [ -d /run/systemd/system ]; then
    echo -n systemd
    exit 0
elif [ -f /sbin/initctl ] && /sbin/initctl --system list 2>&1 > /dev/null; then
    echo -n upstart
    exit 0
fi

# uh-oh
exit 1
`

// DiscoverInitSystemScript returns the shell script to use when
// discovering the local init system. The script is quite specific to
// bash, so it includes an explicit bash shbang.
func DiscoverInitSystemScript() string {
	renderer := shell.BashRenderer{}
	data := renderer.RenderScript([]string{discoverInitSystemScript})
	return string(data)
}

// shellCase is the template for a bash case statement, for use in
// newShellSelectCommand.
const shellCase = `
case "$%s" in
%s
*)
    %s
    ;;
esac`

// newShellSelectCommand creates a bash case statement with clause for
// each of the linux init systems. The body of each clause comes from
// calling the provided handler with the init system name. If the
// handler does not support the args then it returns a false "ok" value.
func newShellSelectCommand(envVarName, dflt string, handler func(string) (string, bool)) string {
	var cases []string
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

	return fmt.Sprintf(shellCase[1:], envVarName, strings.Join(cases, "\n"), dflt)
}
