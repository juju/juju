// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor118 returns upgrade steps that manipulate state directly for Juju 1.18.
func stateStepsFor118() []StateStep {
	return []StateStep{
		&stateUpgradeStep{
			description: "update rsyslog port",
			targets:     []Target{StateServer},
			run:         updateRsyslogPort,
		},
		&stateUpgradeStep{
			description: "remove deprecated environment config settings",
			targets:     []Target{StateServer},
			run:         processDeprecatedEnvSettings,
		},
		&stateUpgradeStep{
			description: "migrate local provider agent config",
			targets:     []Target{StateServer},
			run:         migrateLocalProviderAgentConfig,
		},
	}
}

// stepsFor118 returns upgrade steps for Juju 1.18.
func stepsFor118() []Step {
	return []Step{
		&upgradeStep{
			description: "make $DATADIR/locks owned by ubuntu:ubuntu",
			targets:     []Target{AllMachines},
			run:         ensureLockDirExistsAndUbuntuWritable,
		},
		&upgradeStep{
			description: "generate system ssh key",
			targets:     []Target{StateServer},
			run:         ensureSystemSSHKey,
		},
		&upgradeStep{
			description: "install rsyslog-gnutls",
			targets:     []Target{AllMachines},
			run:         installRsyslogGnutls,
		},
		&upgradeStep{
			description: "make /home/ubuntu/.profile source .juju-proxy file",
			targets:     []Target{AllMachines},
			run:         ensureUbuntuDotProfileSourcesProxyFile,
		},
	}
}
