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
	"Action":                       0,
	"Addresser":                    1,
	"Agent":                        1,
	"AllWatcher":                   0,
	"AllEnvWatcher":                1,
	"Annotations":                  1,
	"Backups":                      0,
	"Block":                        1,
	"Charms":                       1,
	"CharmRevisionUpdater":         0,
	"Client":                       0,
	"Cleaner":                      1,
	"Deployer":                     0,
	"DiskManager":                  1,
	"EntityWatcher":                1,
	"Environment":                  0,
	"EnvironmentManager":           1,
	"FilesystemAttachmentsWatcher": 1,
	"Firewaller":                   1,
	"HighAvailability":             1,
	"ImageManager":                 1,
	"ImageMetadata":                1,
	"InstancePoller":               1,
	"KeyManager":                   0,
	"KeyUpdater":                   0,
	"LeadershipService":            1,
	"Logger":                       0,
	"MachineManager":               1,
	"Machiner":                     0,
	"MetricsManager":               0,
	"Networker":                    0,
	"NotifyWatcher":                0,
	"Pinger":                       0,
	"Provisioner":                  1,
	"Reboot":                       1,
	"RelationUnitsWatcher":         0,
	"Resumer":                      1,
	"Rsyslog":                      0,
	"Service":                      1,
	"Storage":                      1,
	"Spaces":                       1,
	"Subnets":                      1,
	"StorageProvisioner":           1,
	"StringsWatcher":               0,
	"SystemManager":                1,
	"Upgrader":                     0,
	"Uniter":                       2,
	"UserManager":                  0,
	"VolumeAttachmentsWatcher":     1,
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
