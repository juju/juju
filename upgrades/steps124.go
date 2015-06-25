// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
)

// stateStepsFor124 returns upgrade steps for Juju 1.24 that manipulate state directly.
func stateStepsFor124() []Step {
	return []Step{
		&upgradeStep{
			description: "add block device documents for existing machines",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddDefaultBlockDevicesDocs(context.State())
			},
		}, &upgradeStep{
			description: "add instance id field to IP addresses",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddInstanceIdFieldOfIPAddresses(context.State())
			},
		},
	}
}

// stepsFor124 returns upgrade steps for Juju 1.24 that only need the API.
func stepsFor124() []Step {
	return []Step{
		&upgradeStep{
			description: "move syslog config from LogDir to ConfDir",
			targets:     []Target{AllMachines},
			run:         moveSyslogConfig,
		},
	}
}

func moveSyslogConfig(context Context) error {
	config := context.AgentConfig()
	namespace := config.Value(agent.Namespace)
	logdir := config.LogDir()
	confdir := agent.DefaultConfDir
	if namespace != "" {
		confdir += "-" + namespace
	}

	// these values were copied from
	// github.com/juju/juju/utils/syslog/config.go
	// Yes this is bad, but it is only needed once every for an
	// upgrade, so it didn't seem worth exporting those values.
	files := []string{
		"ca-cert.pem",
		"rsyslog-cert.pem",
		"rsyslog-key.pem",
		"logrotate.conf",
		"logrotate.run",
	}
	var errs []string
	for _, f := range files {
		oldpath := filepath.Join(logdir, f)
		newpath := filepath.Join(confdir, f)
		if err := copyFile(newpath, oldpath); errors.IsNotFound(err) {
			continue
		} else if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := osRemove(oldpath); err != nil {
			// Don't fail the step if we can't get rid of the old files.
			// We don't actually care if they still exist or not.
			logger.Warningf("Can't delete old config file %q: %s", oldpath, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("error(s) while moving old syslog config files: %s", strings.Join(errs, "\n"))
	}
	return nil
}

// for testing... of course.
var osRemove = os.Remove

// copyFile copies a file from one location to another.  It won't overwrite
// existing files and will return nil in this case.  This is used instead of
// os.Rename because os.Rename won't work across partitions.
func copyFile(to, from string) error {
	logger.Debugf("Copying %q to %q", from, to)
	orig, err := os.Open(from)
	if os.IsNotExist(err) {
		logger.Debugf("Old file %q does not exist, skipping.", from)
		// original doesn't exist, that's fine.
		return errors.NotFoundf(from)
	}
	if err != nil {
		return errors.Trace(err)
	}
	defer orig.Close()
	info, err := orig.Stat()
	if err != nil {
		return errors.Trace(err)
	}
	target, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_EXCL, info.Mode())
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	defer target.Close()
	if _, err := io.Copy(target, orig); err != nil {
		return errors.Trace(err)
	}
	return nil
}
