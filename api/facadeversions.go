// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// facadeVersions lists the best version of facades that we know about. This
// will be used to pick out a default version for communication, given the list
// of known versions that the API server tells us it is capable of supporting.
// This map should be updated whenever the API server exposes a new version (so
// that the client will use it whenever it is available).
// New facades should start at 1.
// Facades that existed before versioning start at 0.
var facadeVersions = map[string]int{
	"Action":                       6,
	"ActionPruner":                 1,
	"Agent":                        2,
	"AgentTools":                   1,
	"AllModelWatcher":              2,
	"AllWatcher":                   1,
	"Annotations":                  2,
	"Application":                  11,
	"ApplicationOffers":            2,
	"ApplicationScaler":            1,
	"Backups":                      2,
	"Block":                        2,
	"Bundle":                       4,
	"CAASAgent":                    1,
	"CAASAdmission":                1,
	"CAASFirewaller":               1,
	"CAASOperator":                 1,
	"CAASOperatorProvisioner":      1,
	"CAASOperatorUpgrader":         1,
	"CAASUnitProvisioner":          1,
	"CharmRevisionUpdater":         2,
	"Charms":                       2,
	"Cleaner":                      2,
	"Client":                       2,
	"Cloud":                        7,
	"Controller":                   9,
	"CredentialManager":            1,
	"CredentialValidator":          2,
	"CrossController":              1,
	"CrossModelRelations":          2,
	"Deployer":                     1,
	"DiskManager":                  2,
	"EntityWatcher":                2,
	"ExternalControllerUpdater":    1,
	"FanConfigurer":                1,
	"FilesystemAttachmentsWatcher": 2,
	"Firewaller":                   5,
	"FirewallRules":                1,
	"HighAvailability":             2,
	"HostKeyReporter":              1,
	"ImageManager":                 2,
	"ImageMetadata":                3,
	"ImageMetadataManager":         1,
	"InstanceMutater":              2,
	"InstancePoller":               4,
	"KeyManager":                   1,
	"KeyUpdater":                   1,
	"LeadershipService":            2,
	"LifeFlag":                     1,
	"LogForwarding":                1,
	"Logger":                       1,
	"MachineActions":               1,
	"MachineManager":               6,
	"MachineUndertaker":            1,
	"Machiner":                     2,
	"MeterStatus":                  2,
	"MetricsAdder":                 2,
	"MetricsDebug":                 2,
	"MetricsManager":               1,
	"MigrationFlag":                1,
	"MigrationMaster":              2,
	"MigrationMinion":              1,
	"MigrationStatusWatcher":       1,
	"MigrationTarget":              1,
	"ModelConfig":                  2,
	"ModelGeneration":              4,
	"ModelManager":                 8,
	"ModelSummaryWatcher":          1,
	"ModelUpgrader":                1,
	"NotifyWatcher":                1,
	"OfferStatusWatcher":           1,
	"Payloads":                     1,
	"PayloadsHookContext":          1,
	"Pinger":                       1,
	"Provisioner":                  10,
	"ProxyUpdater":                 2,
	"Reboot":                       2,
	"RelationStatusWatcher":        1,
	"RelationUnitsWatcher":         1,
	"RemoteRelations":              2,
	"RemoteRelationWatcher":        1,
	"Resources":                    1,
	"ResourcesHookContext":         1,
	"Resumer":                      2,
	"RetryStrategy":                1,
	"Singular":                     2,
	"Spaces":                       6,
	"SSHClient":                    2,
	"StatusHistory":                2,
	"Storage":                      6,
	"StorageProvisioner":           4,
	"StringsWatcher":               1,
	"Subnets":                      4,
	"Undertaker":                   1,
	"UnitAssigner":                 1,
	"Uniter":                       15,
	"Upgrader":                     1,
	"UpgradeSeries":                1,
	"UpgradeSteps":                 2,
	"UserManager":                  2,
	"VolumeAttachmentsWatcher":     2,
	"VolumeAttachmentPlansWatcher": 1,
}

// bestVersion tries to find the newest version in the version list that we can
// use.
func bestVersion(desiredVersion int, versions []int) int {
	best := 0
	for _, version := range versions {
		if version <= desiredVersion && version > best {
			best = version
		}
	}
	return best
}
