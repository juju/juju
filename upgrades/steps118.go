// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stepsFor118 returns upgrade steps to upgrade to a Juju 1.18 deployment.
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
			description: "update rsyslog port",
			targets:     []Target{StateServer},
			run:         updateRsyslogPort,
		},
		&upgradeStep{
			description: "install rsyslog-gnutls",
			targets:     []Target{AllMachines},
			run:         installRsyslogGnutls,
		},
		&upgradeStep{
			description: "remove deprecated environment config settings",
			targets:     []Target{StateServer},
			run:         processDeprecatedEnvSettings,
		},
		&upgradeStep{
			description: "migrate local provider agent config",
			targets:     []Target{StateServer},
			run:         migrateLocalProviderAgentConfig,
		},
		&upgradeStep{
			description: "make /home/ubuntu/.profile source .juju-proxy file",
			targets:     []Target{AllMachines},
			run:         ensureUbuntuDotProfileSourcesProxyFile,
		},
	}
}
