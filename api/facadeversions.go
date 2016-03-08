// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"
)

// facadeVersions lists the best version of facades that we know about. This
// will be used to pick out a default version for communication, given the list
// of known versions that the API server tells us it is capable of supporting.
// This map should be updated whenever the API server exposes a new version (so
// that the client will use it whenever it is available).
// New facades should start at 1.
// Facades that existed before versioning start at 0.
var facadeVersions = map[string]int{
	"Action":                       1,
	"Addresser":                    2,
	"Agent":                        2,
	"AgentTools":                   1,
	"AllWatcher":                   1,
	"AllModelWatcher":              2,
	"Annotations":                  2,
	"Backups":                      1,
	"Block":                        2,
	"Charms":                       2,
	"CharmRevisionUpdater":         1,
	"Client":                       1,
	"Cleaner":                      2,
	"Controller":                   2,
	"Deployer":                     1,
	"DiscoverSpaces":               2,
	"DiskManager":                  2,
	"EntityWatcher":                2,
	"FilesystemAttachmentsWatcher": 2,
	"Firewaller":                   2,
	"HighAvailability":             2,
	"ImageManager":                 2,
	"ImageMetadata":                2,
	"InstancePoller":               2,
	"KeyManager":                   1,
	"KeyUpdater":                   1,
	"LeadershipService":            2,
	"Logger":                       1,
	"MachineManager":               2,
	"Machiner":                     1,
	"MetricsDebug":                 1,
	"MetricsManager":               1,
	"MeterStatus":                  1,
	"MetricsAdder":                 2,
	"ModelManager":                 2,
	"NotifyWatcher":                1,
	"Pinger":                       1,
	"Provisioner":                  2,
	"ProxyUpdater":                 1,
	"Reboot":                       2,
	"RelationUnitsWatcher":         1,
	"Resumer":                      2,
	"RetryStrategy":                1,
	"Service":                      3,
	"Storage":                      2,
	"Spaces":                       2,
	"Subnets":                      2,
	"StatusHistory":                2,
	"StorageProvisioner":           2,
	"StringsWatcher":               1,
	"Upgrader":                     1,
	"UnitAssigner":                 1,
	"Uniter":                       3,
	"UserManager":                  1,
	"VolumeAttachmentsWatcher":     2,
	"Undertaker":                   1,
}

// RegisterFacadeVersion sets the API client to prefer the given version
// for the facade.
func RegisterFacadeVersion(name string, version int) error {
	if ver, ok := facadeVersions[name]; ok && ver != version {
		return errors.Errorf("facade %q already registered", name)
	}
	facadeVersions[name] = version
	return nil
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
