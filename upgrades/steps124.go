// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
			description: "move syslog config from LogDir to DataDir",
			targets:     []Target{AllMachines},
			run: func(context Context) error {
				config := context.AgentConfig()
				logdir := config.LogDir()
				datadir := config.DataDir()

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
					newpath := filepath.Join(datadir, f)
					if err := copy(newpath, oldpath); err != nil {
						errs = append(errs, err.Error())
						continue
					}
					if err := os.Remove(oldpath); err != nil {
						errs = append(errs, err.Error())
					}
				}
				if len(errs) > 0 {
					return fmt.Errorf("error(s) while moving old syslog config files: ", strings.Join(errs, "\n"))
				}
				return nil
			},
		},
	}
}

func copy(to, from string) error {
	logger.Debugf("Moving %q to %q", from, to)
	orig, err := os.Open(from)
	if os.IsNotExist(err) {
		logger.Debugf("Old file %q does not exist, skipping.", from)
		// original doesn't exist, that's fine.
		return nil
	}
	if err != nil {
		return err
	}
	defer orig.Close()
	info, err := orig.Stat()
	if err != nil {
		return err
	}
	target, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY, info.Mode())
	if os.IsExist(err) {
		logger.Debugf("New file %q already exists, deleting old file %q", to, from)
		// don't overwrite an existing target
		orig.Close()

		// this may fail, but there's not much we can do about it.
		_ = os.Remove(from)
		return nil
	}
	defer target.Close()
	if _, err := io.Copy(target, orig); err != nil {
		return err
	}
	return nil
}
