// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stepsFor118 returns upgrade steps to upgrade to a Juju 1.18 deployment.
func stepsFor118() []UpgradeStep {
	return []UpgradeStep{
		&upgradeStep{
			description: "make $DATADIR/locks owned by ubuntu:ubuntu",
			targets:     []UpgradeTarget{HostMachine},
			run:         ensureLockDirExistsAndUbuntuWritable,
		},
		&upgradeStep{
			description: "upgrade rsyslog config file on state server",
			targets:     []UpgradeTarget{StateServer},
			run:         upgradeStateServerRsyslogConfig,
		},
		&upgradeStep{
			description: "upgrade rsyslog config file on host machine",
			targets:     []UpgradeTarget{HostMachine},
			run:         upgradeHostMachineRsyslogConfig,
		},
	}
}
