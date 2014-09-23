// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

var (
	UpgradeOperations         = &upgradeOperations
	UbuntuHome                = &ubuntuHome
	RootLogDir                = &rootLogDir
	RootSpoolDir              = &rootSpoolDir
	CharmBundleURL            = &charmBundleURL
	CharmStoragePath          = &charmStoragePath
	StateAddCharmStoragePaths = &stateAddCharmStoragePaths
	StateStorage              = &stateStorage

	ChownPath      = &chownPath
	IsLocalEnviron = &isLocalEnviron

	// 118 upgrade functions
	StepsFor118                            = stepsFor118
	EnsureLockDirExistsAndUbuntuWritable   = ensureLockDirExistsAndUbuntuWritable
	EnsureSystemSSHKey                     = ensureSystemSSHKey
	EnsureUbuntuDotProfileSourcesProxyFile = ensureUbuntuDotProfileSourcesProxyFile
	UpdateRsyslogPort                      = updateRsyslogPort
	ProcessDeprecatedEnvSettings           = processDeprecatedEnvSettings
	MigrateLocalProviderAgentConfig        = migrateLocalProviderAgentConfig

	// 121 upgrade functions
	StepsFor121a1              = stepsFor121a1
	StepsFor121a2              = stepsFor121a2
	MigrateCharmStorage        = migrateCharmStorage
	MigrateCustomImageMetadata = migrateCustomImageMetadata
)
