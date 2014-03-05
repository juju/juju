// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

var (
	UpgradeOperations = &upgradeOperations
	UbuntuHome        = &ubuntuHome

	// 118 upgrade functions
	StepsFor118                          = stepsFor118
	EnsureLockDirExistsAndUbuntuWritable = ensureLockDirExistsAndUbuntuWritable
	EnsureSystemSSHKey                   = ensureSystemSSHKey
	UpdateRsyslogPort                    = updateRsyslogPort
	ProcessDeprecatedAttributes          = processDeprecatedAttributes
)
