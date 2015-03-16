package service

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/version"
)

// This exists to allow patching during tests.
var getVersion = func() version.Binary {
	return version.Current
}

// DiscoverService returns an interface to a service apropriate
// for the current system
func DiscoverService(name string, conf common.Conf) (Service, error) {
	initName, err := discoverLocalInitSystem()
	if errors.IsNotFound(err) {
		// Fall back to checking the juju version.
		jujuVersion := getVersion()
		versionInitName, ok := VersionInitSystem(jujuVersion)
		if !ok {
			// The key error is the one from discoverLocalInitSystem so
			// that is what we return.
			return nil, errors.Trace(err)
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
		// TODO(ericsnow) Support other OSes, like version.CentOS.
	default:
		return "", false
	}
}

// pid1 is the path to the "file" that contains the path to the init
// system executable on linux.
const pid1 = "/proc/1/cmdline"

// These exist to allow patching during tests.
var (
	runtimeOS    = func() string { return runtime.GOOS }
	pid1Filename = func() string { return pid1 }
	osStat       = os.Stat
	osReadlink   = os.Readlink

	initExecutable = func() (string, error) {
		pid1File := pid1Filename()
		data, err := ioutil.ReadFile(pid1File)
		if os.IsNotExist(err) {
			return "", errors.NotFoundf("init system (via %q)", pid1File)
		}
		if err != nil {
			return "", errors.Annotatef(err, "failed to identify init system (via %q)", pid1File)
		}
		executable := strings.Split(string(data), "\x00")[0]
		return executable, nil
	}
)

func discoverLocalInitSystem() (string, error) {
	if runtimeOS() == "windows" {
		return InitSystemWindows, nil
	}

	executable, err := initExecutable()
	if err != nil {
		return "", errors.Trace(err)
	}

	followLink := true
	initName, ok := identifyInitSystem(executable, followLink)
	if !ok {
		return "", errors.NotFoundf("init system (based on %q)", executable)
	}
	logger.Debugf("discovered init system %q from executable %q", initName, executable)
	return initName, nil
}

func identifyInitSystem(executable string, followLink bool) (string, bool) {
	initSystem, ok := identifyExecutable(executable)
	if ok {
		return initSystem, true
	}

	finfo, err := osStat(executable)
	if os.IsNotExist(err) {
		return "", false
	} else if err != nil {
		logger.Errorf("failed to find %q: %v", executable, err)
		// The stat check is just an optimization so we go on anyway.
	}

	// First fall back to following symlinks.
	if followLink && (finfo.Mode()&os.ModeSymlink) != 0 {
		linked, err := osReadlink(executable)
		if err != nil {
			logger.Errorf("could not follow link %q (%v)", executable, err)
			// TODO(ericsnow) Try checking the version anyway?
			return "", false
		}
		// We do not follow any more links since we want to avoid
		// infinite recursion and it's unlikely the original link
		// points to another link.
		followLink = false
		return identifyInitSystem(linked, followLink)
	}

	// Fall back to checking the "version" text.
	cmd := exec.Command(executable, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Errorf(`"%s --version" failed (%v): %s`, executable, err, out)
		return "", false
	}

	verText := string(out)
	switch {
	case strings.Contains(verText, "upstart"):
		return InitSystemUpstart, true
	case strings.Contains(verText, "systemd"):
		return InitSystemSystemd, true
	}

	// uh-oh
	return "", false
}

func identifyExecutable(executable string) (string, bool) {
	switch {
	case strings.Contains(executable, "upstart"):
		return InitSystemUpstart, true
	case strings.Contains(executable, "systemd"):
		return InitSystemSystemd, true
	default:
		return "", false
	}
}

// TODO(ericsnow) Synchronize newShellSelectCommand with discoverLocalInitSystem.

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
