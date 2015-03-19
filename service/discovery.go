package service

import (
	"fmt"
	"os/exec"
	"path/filepath"
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
		// TODO(ericsnow) Support other OSes, like version.CentOS.
	default:
		return "", false
	}
}

// These exist to allow patching during tests.
var (
	runtimeOS    = func() string { return runtime.GOOS }
	evalSymlinks = filepath.EvalSymlinks
	psPID1       = func() ([]byte, error) {
		cmd := exec.Command("/bin/ps", "-p", "1", "-o", "cmd", "--no-headers")
		return cmd.Output()
	}

	initExecutable = func() (string, error) {
		psOutput, err := psPID1()
		if err != nil {
			return "", errors.Annotate(err, "failed to identify init system using ps")
		}
		return strings.Fields(string(psOutput))[0], nil
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

	initName, ok := identifyInitSystem(executable)
	if !ok {
		return "", errors.NotFoundf("init system (based on %q)", executable)
	}
	logger.Debugf("discovered init system %q from executable %q", initName, executable)
	return initName, nil
}

func identifyInitSystem(executable string) (string, bool) {
	initSystem, ok := identifyExecutable(executable)
	if ok {
		return initSystem, true
	}

	// First fall back to following symlinks (if any).
	resolved, err := evalSymlinks(executable)
	if err != nil {
		logger.Errorf("failed to find %q: %v", executable, err)
		return "", false
	}
	executable = resolved
	initSystem, ok = identifyExecutable(executable)
	if ok {
		return initSystem, true
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

// TODO(ericsnow) Build this script more dynamically (using shell.Renderer).
// TODO(ericsnow) Use a case statement in the script?

// DiscoverInitSystemScript is the shell script to use when
// discovering the local init system.
const DiscoverInitSystemScript = `#!/usr/bin/env bash

function checkInitSystem() {
    if [[ $1 == *"systemd"* ]]; then
        echo -n systemd
        exit $?
    elif [[ $1 == *"upstart"* ]]; then
        echo -n upstart
        exit $?
    fi
}

# Find the executable.
executable=$(ps -p 1 -o cmd --no-headers | awk '{print $1}')
if [[ $? -ne 0 ]]; then
    exit 1
fi

# Check the executable.
checkInitSystem "$executable"

# First fall back to following symlinks.
if [[ -L $executable ]]; then
    linked=$(readlink -f "$executable")
    if [[ $? -eq 0 ]]; then
        executable=$linked

        # Check the linked executable.
        checkInitSystem "$linked"
    fi
fi

# Fall back to checking the "version" text.
verText=$("${executable}" --version)
if [[ $? -eq 0 ]]; then
    checkInitSystem "$verText"
fi

# uh-oh
exit 1
`

func writeDiscoverInitSystemScript(filename string) []string {
	// TODO(ericsnow) Use utils.shell.Renderer.WriteScript.
	return []string{
		fmt.Sprintf(`
cat > %s << 'EOF'
%s
EOF`[1:], filename, DiscoverInitSystemScript),
		"chmod 0755 " + filename,
	}
}

const caseLine = "%sif [[ $%s == \"%s\" ]]; then %s\n"

// newShellSelectCommand creates a bash if statement with an if
// (or elif) clause for each of the executables in linuxExecutables.
// The body of each clause comes from calling the provided handler with
// the init system name. If the handler does not support the args then
// it returns a false "ok" value.
func newShellSelectCommand(envVarName string, handler func(string) (string, bool)) string {
	// TODO(ericsnow) Build the command in a better way?
	// TODO(ericsnow) Use a case statement?

	prefix := ""
	lines := ""
	for _, initSystem := range linuxInitSystems {
		cmd, ok := handler(initSystem)
		if !ok {
			continue
		}
		lines += fmt.Sprintf(caseLine, prefix, envVarName, initSystem, cmd)

		if prefix != "el" {
			prefix = "el"
		}
	}
	if lines != "" {
		lines += "" +
			"else exit 1\n" +
			"fi"
	}
	return lines
}
