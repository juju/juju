// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

var (
	UpgradeOperations         = &upgradeOperations
	StateUpgradeOperations    = &stateUpgradeOperations
	UbuntuHome                = &ubuntuHome
	RootLogDir                = &rootLogDir
	RootSpoolDir              = &rootSpoolDir
	CharmBundleURL            = &charmBundleURL
	CharmStoragePath          = &charmStoragePath
	StateAddCharmStoragePaths = &stateAddCharmStoragePaths
	StateStorage              = &stateStorage
	StateToolsStorage         = &stateToolsStorage

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
	MigrateCharmStorage        = migrateCharmStorage
	MigrateCustomImageMetadata = migrateCustomImageMetadata
	MigrateToolsStorage        = migrateToolsStorage

	// 122 upgrade functions
	EnsureSystemSSHKeyRedux               = ensureSystemSSHKeyRedux
	UpdateAuthorizedKeysForSystemIdentity = updateAuthorizedKeysForSystemIdentity
)
