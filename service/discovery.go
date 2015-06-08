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

// DiscoverService returns an interface to a service apropriate
// for the current system
func DiscoverService(name string, conf common.Conf) (Service, error) {
	initName, err := discoverInitSystem()
	if err != nil {
		return nil, errors.Trace(err)
	}

	service, err := NewService(name, conf, initName)
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
		versionInitName, ok := VersionInitSystem(jujuVersion)
		if !ok {
			// The key error is the one from discoverLocalInitSystem so
			// that is what we return.
			return "", errors.Trace(err)
		}
		initName = versionInitName
	} else if err != nil {
		return "", errors.Trace(err)
	}
	return initName, nil
}

// VersionInitSystem returns an init system name based on the provided
// version info. If one cannot be identified then false if returned
// for the second return value.
func VersionInitSystem(vers version.Binary) (string, bool) {
	initName, ok := versionInitSystem(vers)
	if !ok {
		logger.Errorf("could not identify init system from juju version info (%#v)", vers)
		return "", false
	}
	logger.Debugf("discovered init system %q from juju version info (%#v)", initName, vers)
	return initName, true
}

func versionInitSystem(vers version.Binary) (string, bool) {
	switch vers.OS {
	case version.Windows:
		return InitSystemWindows, true
	case version.Ubuntu:
		switch vers.Series {
		case "precise", "quantal", "raring", "saucy", "trusty", "utopic":
			return InitSystemUpstart, true
		case "":
			return "", false
		default:
			// Check for pre-precise releases.
			os, _ := version.GetOSFromSeries(vers.Series)
			if os == version.Unknown {
				return "", false
			}
			// vivid and later
			if featureflag.Enabled(feature.LegacyUpstart) {
				return InitSystemUpstart, true
			}
			return InitSystemSystemd, true
		}
	case version.CentOS:
		return InitSystemSystemd, true
	default:
		return "", false
	}
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
