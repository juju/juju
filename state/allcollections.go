// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/mgo/v3"
)

// allCollections should be the single source of truth for information about
// any collection we use. It's broken up into 4 main sections:
//
//   - infrastructure: we really don't have any business touching these once
//     we've created them. They should have the rawAccess attribute set, so that
//     multiModelRunner will consider them forbidden.
//
//   - global: these hold information external to models. They may include
//     model metadata, or references; but they're generally not relevant
//     from the perspective of a given model.
//
//   - local (in opposition to global; and for want of a better term): these
//     hold information relevant *within* specific models (machines,
//     applications, relations, settings, bookkeeping, etc) and should generally be
//     read via an modelStateCollection, and written via a multiModelRunner. This is
//     the most common form of collection, and the above access should usually
//     be automatic via Database.Collection and Database.Runner.
//
//   - raw-access: there's certainly data that's a poor fit for mgo/txn. Most
//     forms of logs, for example, will benefit both from the speedy insert and
//     worry-free bulk deletion; so raw-access collections are fine. Just don't
//     try to run transactions that reference them.
//
// Please do not use collections not referenced here; and when adding new
// collections, please document them, and make an effort to put them in an
// appropriate section.
func allCollections() CollectionSchema {
	result := CollectionSchema{

		// Infrastructure collections
		// ==========================

		globalClockC: {
			global:    true,
			rawAccess: true,
		},
		txnsC: {
			// This collection is used exclusively by mgo/txn to record transactions.
			global:    true,
			rawAccess: true,
			indexes: []mgo.Index{{
				// The "s" field is used in queries
				// by mgo/txn.Runner.ResumeAll.
				Key: []string{"s"},
			}},
		},

		// ------------------

		// Global collections
		// ==================

		// This collection holds the details of the controllers hosting, well,
		// everything in state.
		controllersC: {global: true},

		// This collection holds the details of the HA-ness of controllers.
		controllerNodesC: {},

		// This collection is used by the controllers to coordinate binary
		// upgrades and schema migrations.
		upgradeInfoC: {global: true},

		// This collection holds a convenient representation of the content of
		// the simplestreams data source pointing to binaries required by juju.
		//
		// Tools metadata is per-model, to allow multiple revisions of tools to
		// be uploaded to different models without affecting other models.
		toolsmetadataC: {},

		// This collection holds model information; in particular its
		// Life and its UUID.
		modelsC: {
			global: true,
			indexes: []mgo.Index{{
				Key:    []string{"name", "owner"},
				Unique: true,
			}},
		},

		// This collection holds references to entities owned by a
		// model. We use this to determine whether or not we can safely
		// destroy empty models.
		modelEntityRefsC: {global: true},

		// This collection is holds the parameters for model migrations.
		migrationsC: {
			global: true,
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "-attempt"},
			}},
		},

		// This collection tracks the progress of model migrations.
		migrationsStatusC: {global: true},

		// This collection holds messages about the progress of model
		// migrations. It is split from migrationsStatusC to prevent the
		// messages triggering the status watcher.
		migrationsStatusMessageC: {global: true},

		// This collection records the model migrations which
		// are currently in progress. It is used to ensure that only
		// one model migration document exists per model.
		migrationsActiveC: {global: true},

		// This collection tracks migration progress reports from the
		// migration minions.
		migrationsMinionSyncC: {global: true},

		// This collection is used as a unique key restraint. The _id field is
		// a concatenation of multiple fields that form a compound index,
		// allowing us to ensure users cannot have the same name for two
		// different models at a time.
		usermodelnameC: {global: true},

		// This collection holds settings from various sources which
		// are inherited and then forked by new models.
		globalSettingsC: {global: true},

		// This collection was deprecated before multi-model support
		// was implemented.
		actionresultsC: {global: true},

		// This collection holds storage items for a macaroon bakery.
		bakeryStorageItemsC: {
			global: true,
		},

		// -----------------

		// Local collections
		// =================

		// This collection contains governors that prevent certain kinds of
		// changes from being accepted.
		blocksC: {},

		// This collection is used for internal bookkeeping; certain complex
		// or tedious state changes are deferred by recording a cleanup doc
		// for later handling.
		cleanupsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// This collection contains incrementing integers, subdivided by name,
		// to ensure various IDs aren't reused.
		sequenceC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// -----

		// These collections hold information associated with applications.
		applicationsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "name"},
			}},
		},
		unitsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "application"},
			}, {
				Key: []string{"model-uuid", "principal"},
			}, {
				Key: []string{"model-uuid", "machineid"},
			}, {
				Key: []string{"model-uuid", "name"},
			}},
		},
		unitStatesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// This collection holds documents that indicate units which are queued
		// to be assigned to machines. It is used exclusively by the
		// AssignUnitWorker.
		assignUnitC: {},

		// These collections hold reference counts which are used
		// by the nsRefcounts struct.
		refcountsC: {}, // Per model.
		globalRefcountsC: {
			global: true,
		},

		relationsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "endpoints.applicationname", "endpoints.relation.name"},
			}, {
				Key: []string{"model-uuid", "id"}, // id here is the relation id not the doc _id
			}},
		},
		relationScopesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "key", "departing"},
			}},
		},

		// -----

		// These collections hold information associated with machines.
		containerRefsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},
		machinesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "machineid"},
			}},
		},
		sshHostKeysC: {},

		// This collection contains information from removed machines
		// that needs to be cleaned up in the provider.
		machineRemovalsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// -----

		filesystemsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "storageid"},
			}, {
				Key: []string{"model-uuid", "machineid"},
			}},
		},
		filesystemAttachmentsC: {},
		storageInstancesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "owner"},
			}},
		},
		storageAttachmentsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "storageid"},
			}, {
				Key: []string{"model-uuid", "unitid"},
			}},
		},
		volumesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "storageid"},
			}, {
				Key: []string{"model-uuid", "hostid"},
			}},
		},
		volumeAttachmentsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "hostid"},
			}, {
				Key: []string{"model-uuid", "volumeid"},
			}},
		},
		volumeAttachmentPlanC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// -----

		providerIDsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},
		linkLayerDevicesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "machine-id"},
			}},
		},
		ipAddressesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "machine-id", "device-name"},
			}},
		},
		endpointBindingsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// -----

		// These collections hold information associated with actions.
		actionsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "name"},
			}, {
				Key: []string{"model-uuid", "operation"},
			}},
		},
		actionNotificationsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},
		operationsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "_id"},
			}},
		},

		// -----

		// The remaining non-global collections share the property of being
		// relevant to multiple other kinds of entities, and are thus generally
		// indexed by globalKey(). This is unhelpfully named in this context --
		// it's meant to imply "global within an model", because it was
		// named before multi-model support.

		// This collection in particular holds an astounding number of
		// different sorts of data: application config settings by charm version,
		// unit relation settings, model config, etc etc etc.
		settingsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		constraintsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},
		storageConstraintsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},
		statusesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "_id"},
			}},
		},

		// cloudContainersC holds the CAAS container (pod) information
		// for units, eg address, ports.
		cloudContainersC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "provider-id"},
			}},
		},

		// cloudServicesC holds the CAAS service information
		// eg addresses.
		cloudServicesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},
	}
	return result
}

// These constants are used to avoid sprinkling the package with any more
// magic strings. If a collection deserves documentation, please document
// it in allCollections, above; and please keep this list sorted for easy
// inspection.
const (
	actionNotificationsC     = "actionnotifications"
	actionresultsC           = "actionresults"
	actionsC                 = "actions"
	assignUnitC              = "assignUnits"
	bakeryStorageItemsC      = "bakeryStorageItems"
	blocksC                  = "blocks"
	cleanupsC                = "cleanups"
	cloudContainersC         = "cloudcontainers"
	cloudServicesC           = "cloudservices"
	constraintsC             = "constraints"
	containerRefsC           = "containerRefs"
	controllersC             = "controllers"
	controllerNodesC         = "controllerNodes"
	filesystemAttachmentsC   = "filesystemAttachments"
	filesystemsC             = "filesystems"
	globalClockC             = "globalclock"
	globalRefcountsC         = "globalRefcounts"
	globalSettingsC          = "globalSettings"
	machinesC                = "machines"
	machineRemovalsC         = "machineremovals"
	migrationsActiveC        = "migrations.active"
	migrationsC              = "migrations"
	migrationsMinionSyncC    = "migrations.minionsync"
	migrationsStatusC        = "migrations.status"
	modelsC                  = "models"
	modelEntityRefsC         = "modelEntityRefs"
	operationsC              = "operations"
	providerIDsC             = "providerIDs"
	relationScopesC          = "relationscopes"
	relationsC               = "relations"
	sequenceC                = "sequence"
	applicationsC            = "applications"
	endpointBindingsC        = "endpointbindings"
	settingsC                = "settings"
	refcountsC               = "refcounts"
	sshHostKeysC             = "sshhostkeys"
	statusesC                = "statuses"
	storageAttachmentsC      = "storageattachments"
	storageConstraintsC      = "storageconstraints"
	storageInstancesC        = "storageinstances"
	linkLayerDevicesC        = "linklayerdevices"
	ipAddressesC             = "ip.addresses"
	toolsmetadataC           = "toolsmetadata"
	txnsC                    = "txns"
	migrationsStatusMessageC = "migrations.statusmessage"
	unitsC                   = "units"
	unitStatesC              = "unitstates"
	upgradeInfoC             = "upgradeInfo"
	usermodelnameC           = "usermodelname"
	volumeAttachmentsC       = "volumeattachments"
	volumeAttachmentPlanC    = "volumeattachmentplan"
	volumesC                 = "volumes"
)

// watcherIgnoreList contains all the collections in mongo that should not be watched by the
// TxnWatcher.
var watcherIgnoreList = []string{
	bakeryStorageItemsC,
	sequenceC,
	refcountsC,
}
