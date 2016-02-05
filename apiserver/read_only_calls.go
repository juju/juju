// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/utils/set"
)

// readOnlyCalls specify a white-list of API calls that do not
// modify the database. The format of the calls is "<facade>.<method>".
// At this stage, we are explicitly ignoring the facade version.
var readOnlyCalls = set.NewStrings(
	"Action.Actions",
	"Action.FindActionTagsByPrefix",
	"Action.ListAll",
	"Action.ListPending",
	"Action.ListRunning",
	"Action.ListCompleted",
	"Action.ServicesCharmActions",
	"Annotations.Get",
	"Block.List",
	"Charms.CharmInfo",
	"Charms.IsMetered",
	"Charms.List",
	"Client.AgentVersion",
	"Client.APIHostPorts",
	"Client.CharmInfo",
	"Client.ModelGet",
	"Client.ModelInfo",
	"Client.ModelUserInfo",
	"Client.FullStatus",
	// FindTools, while being technically read only, isn't a useful
	// command for a read only user to run.
	// While GetBundleChanges is technically read only, it is a precursor
	// to deploying the bundle or changes. But... let's leave it here anyway.
	"Client.GetBundleChanges",
	"Client.GetModelConstraints",
	"Client.PrivateAddress",
	"Client.PublicAddress",
	// ResolveCharms, while being technically read only, isn't a useful
	// command for a read only user to run.
	// Status is so old it shouldn't be used.
	"Client.UnitStatusHistory",
	"Client.WatchAll",
	// TODO: add controller work.
	"KeyManager.ListKeys",
	"Service.GetConstraints",
	"Service.CharmRelations",
	"Service.Get",
	"Spaces.ListSpaces",
	"Storage.ListStorageDetails",
	"Storage.ListFilesystems",
	"Storage.ListPools",
	"Storage.ListVolumes",
	"Subnets.AllSpaces",
	"Subnets.AllZones",
	"Subnets.ListSubnets",
	"UserManager.UserInfo",
)

// isCallReadOnly returns whether or not the method on the facade
// is known to not alter the database.
func isCallReadOnly(facade, method string) bool {
	key := facade + "." + method
	// NOTE: maybe useful in the future to be able to specify entire facades
	// as read only, in which case specifying something like "Facade.*" would
	// be useful. Not sure we'll ever need this, but something to think about
	// perhaps.
	return readOnlyCalls.Contains(key)
}
