package service

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/version"
)

var jujuVersion = version.Current

// DiscoverService returns an interface to a service apropriate
// for the current system
func DiscoverService(name string, conf common.Conf) (Service, error) {
	initName, err := discoverLocalInitSystem()
	if errors.IsNotFound(err) {
		// Fall back to checking the juju version.
		versionInitName, ok := VersionInitSystem(jujuVersion)
		if !ok {
			return nil, errors.Annotate(err, "nor on local host")
		}
		initName = versionInitName
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	service, err := NewService(name, conf, initName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return service, nil
}

// VersionInitSystem returns an init system name based on the provided
// version info. If one cannot be identified then false if returned
// for the second return value.
func VersionInitSystem(vers version.Binary) (string, bool) {
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
			return InitSystemSystemd, true
		}
		// TODO(ericsnow) Support other OSes, like version.CentOS.
	default:
		return "", false
	}
}

const pid1 = "/proc/1/cmdline"

type initSystem struct {
	executable string
	name       string
}

var linuxExecutables = []initSystem{
	// Note that some systems link /sbin/init to whatever init system
	// is supported, so in the future we may need some other way to
	// identify upstart uniquely.
	{"/sbin/init", InitSystemUpstart},
	{"/sbin/upstart", InitSystemUpstart},
	{"/sbin/systemd", InitSystemSystemd},
	{"/bin/systemd", InitSystemSystemd},
	{"/lib/systemd/systemd", InitSystemSystemd},
}

func identifyInitSystem(executable string) (string, bool) {
	for _, initSystem := range linuxExecutables {
		if executable == initSystem.executable {
			return initSystem.name, true
		}
	}
	return "", false
}

var runtimeOS = runtime.GOOS
var pid1File = pid1

func discoverLocalInitSystem() (string, error) {
	if runtimeOS == "windows" {
		return InitSystemWindows, nil
	}

	data, err := ioutil.ReadFile(pid1File)
	if os.IsNotExist(err) {
		return "", errors.NotFoundf("init system")
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	out := strings.Trim(strings.TrimSpace(string(data)), "\x00")
	executable := strings.Fields(out)[0]

	initName, ok := identifyInitSystem(executable)
	if !ok {
		return "", errors.NotFoundf("init system (based on %q)", executable)
	}
	logger.Debugf("discovered init system %q from executable %q", initName, executable)
	return initName, nil
}

// TODO(ericsnow) Is it too much to cat once for each executable?
const initSystemTest = `[[ "$(cat ` + pid1 + ` | awk '{print $1}')" == "%s" ]]`

// newShellSelectCommand creates a bash if statement with an if
// (or elif) clause for each of the executables in linuxExecutables.
// The body of each clause comes from calling the provided handler with
// the init system name. If the handler does not support the args then
// it returns a false "ok" value.
func newShellSelectCommand(handler func(string) (string, bool)) string {
	// TODO(ericsnow) Allow passing in "initSystems ...string".
	executables := linuxExecutables

	// TODO(ericsnow) build the command in a better way?

	cmdAll := ""
	for _, initSystem := range executables {
		cmd, ok := handler(initSystem.name)
		if !ok {
			continue
		}

		test := fmt.Sprintf(initSystemTest, initSystem.executable)
		cmd = fmt.Sprintf("if %s; then %s\n", test, cmd)
		if cmdAll != "" {
			cmd = "el" + cmd
		}
		cmdAll += cmd
	}
	if cmdAll != "" {
		cmdAll += "" +
			"else exit 1\n" +
			"fi"
	}
	return cmdAll
}
