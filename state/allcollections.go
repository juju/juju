// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2"
)

// The capped collection used for transaction logs defaults to 10MB.
// It's tweaked in export_test.go to 1MB to avoid the overhead of
// creating and deleting the large file repeatedly in tests.
var (
	txnLogSize      = 10000000
	txnLogSizeTests = 1000000
)

// allCollections should be the single source of truth for information about
// any collection we use. It's broken up into 4 main sections:
//
//  * infrastructure: we really don't have any business touching these once
//    we've created them. They should have the rawAccess attribute set, so that
//    multiModelRunner will consider them forbidden.
//
//  * global: these hold information external to models. They may include
//    model metadata, or references; but they're generally not relevant
//    from the perspective of a given model.
//
//  * local (in opposition to global; and for want of a better term): these
//    hold information relevant *within* specific models (machines,
//    services, relations, settings, bookkeeping, etc) and should generally be
//    read via an modelStateCollection, and written via a multiModelRunner. This is
//    the most common form of collection, and the above access should usually
//    be automatic via Database.Collection and Database.Runner.
//
//  * raw-access: there's certainly data that's a poor fit for mgo/txn. Most
//    forms of logs, for example, will benefit both from the speedy insert and
//    worry-free bulk deletion; so raw-access collections are fine. Just don't
//    try to run transactions that reference them.
//
// Please do not use collections not referenced here; and when adding new
// collections, please document them, and make an effort to put them in an
// appropriate section.
func allCollections() collectionSchema {
	return collectionSchema{

		// Infrastructure collections
		// ==========================

		txnsC: {
			// This collection is used exclusively by mgo/txn to record transactions.
			global:         true,
			rawAccess:      true,
			explicitCreate: &mgo.CollectionInfo{},
		},
		txnLogC: {
			// This collection is used by mgo/txn to record the set of documents
			// affected by each successful transaction; and by state/watcher to
			// generate a stream of document-resolution events that are delivered
			// to, and interpreted by, both state and state/multiwatcher.
			global:    true,
			rawAccess: true,
			explicitCreate: &mgo.CollectionInfo{
				Capped:   true,
				MaxBytes: txnLogSize,
			},
		},

		// ------------------

		// Global collections
		// ==================

		// This collection holds the details of the controllers hosting, well,
		// everything in state.
		controllersC: {global: true},

		// This collection is used to track progress when restoring a
		// controller from backup.
		restoreInfoC: {global: true},

		// This collection is used by the controllers to coordinate binary
		// upgrades and schema migrations.
		upgradeInfoC: {global: true},

		// This collection holds a convenient representation of the content of
		// the simplestreams data source pointing to binaries required by juju.
		toolsmetadataC: {global: true},

		// This collection holds model information; in particular its
		// Life and its UUID.
		modelsC: {global: true},

		// This collection is holds the parameters for model migrations.
		modelMigrationsC: {
			global: true,
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// This collection tracks the progress of model migrations.
		modelMigrationStatusC: {global: true},

		// This collection records the model migrations which
		// are currently in progress. It is used to ensure that only
		// one model migration document exists per environment.
		modelMigrationsActiveC: {global: true},

		// This collection holds user information that's not specific to any
		// one model.
		usersC: {
			global: true,
			indexes: []mgo.Index{{
				// TODO(thumper): schema change to remove this index.
				Key: []string{"name"},
			}},
		},

		// This collection holds the last time the user connected to the API server.
		userLastLoginC: {
			global:    true,
			rawAccess: true,
		},

		// This collection is used as a unique key restraint. The _id field is
		// a concatenation of multiple fields that form a compound index,
		// allowing us to ensure users cannot have the same name for two
		// different models at a time.
		usermodelnameC: {global: true},

		// This collection holds workload metrics reported by certain charms
		// for passing onward to other tools.
		metricsC: {global: true},

		// This collection holds persistent state for the metrics manager.
		metricsManagerC: {global: true},

		// This collection holds lease data, which is per-model, but is
		// not itself multi-model-aware; happily it will imminently be
		// deprecated in favour of the non-global leasesC below.
		// TODO(fwereade): drop leaseC entirely so can't use wrong const.
		leaseC: {global: true},

		// This collection was deprecated before multi-model support
		// was implemented.
		actionresultsC: {global: true},

		// -----------------

		// Local collections
		// =================

		// This collection is basically a standard SQL intersection table; it
		// references the global records of the users allowed access to a
		// given collection.
		modelUsersC: {},

		// This collection holds the last time the model user connected
		// to the model.
		modelUserLastConnectionC: {
			rawAccess: true,
		},

		// This collection contains governors that prevent certain kinds of
		// changes from being accepted.
		blocksC: {},

		// This collection is used for internal bookkeeping; certain complex
		// or tedious state changes are deferred by recording a cleanup doc
		// for later handling.
		cleanupsC: {},

		// This collection contains incrementing integers, subdivided by name,
		// to ensure various IDs aren't reused.
		sequenceC: {},

		// This collection holds lease data. It's currently only used to
		// implement service leadership, but is namespaced and available
		// for use by other clients in future.
		leasesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "type"},
			}, {
				Key: []string{"model-uuid", "namespace"},
			}},
		},

		// -----

		// These collections hold information associated with services.
		charmsC:   {},
		servicesC: {},
		unitsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "service"},
			}, {
				Key: []string{"model-uuid", "principal"},
			}, {
				Key: []string{"model-uuid", "machineid"},
			}},
		},
		minUnitsC: {},

		// This collection holds documents that indicate units which are queued
		// to be assigned to machines. It is used exclusively by the
		// AssignUnitWorker.
		assignUnitC: {},

		// meterStatusC is the collection used to store meter status information.
		meterStatusC:  {},
		settingsrefsC: {},
		relationsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "endpoints.relationname"},
			}, {
				Key: []string{"model-uuid", "endpoints.servicename"},
			}},
		},
		relationScopesC: {},

		// -----

		// These collections hold information associated with machines.
		containerRefsC: {},
		instanceDataC:  {},
		machinesC:      {},
		rebootC:        {},

		// -----

		// These collections hold information associated with storage.
		blockDevicesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "machineid"},
			}},
		},
		filesystemsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "storageid"},
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
			}},
		},
		volumeAttachmentsC: {},

		// -----

		// These collections hold information associated with networking.
		ipaddressesC: {
			indexes: []mgo.Index{{
				Key: []string{"uuid"},
			}, {
				Key: []string{"model-uuid", "state"},
			}, {
				Key: []string{"model-uuid", "subnetid"},
			}},
		},
		networkInterfacesC: {
			indexes: []mgo.Index{{
				Key:    []string{"model-uuid", "interfacename", "machineid"},
				Unique: true,
			}, {
				Key:    []string{"model-uuid", "macaddress", "networkname"},
				Unique: true,
			}, {
				Key: []string{"model-uuid", "machineid"},
			}, {
				Key: []string{"model-uuid", "networkname"},
			}},
		},
		networksC: {
			indexes: []mgo.Index{{
				Key:    []string{"model-uuid", "providerid"},
				Unique: true,
			}},
		},
		openedPortsC:       {},
		requestedNetworksC: {},
		spacesC: {
			indexes: []mgo.Index{{
				// NOTE: Like the DocID field, ProviderId also has the model
				// UUID as prefix to ensure uniqueness per model. However since
				// not all providers support spaces, it can be empty, hence both
				// unique and sparse.
				Key:    []string{"providerid"},
				Unique: true,
				Sparse: true,
			}},
		},
		subnetsC: {
			indexes: []mgo.Index{{
				// NOTE: Like the DocID field, ProviderId also has the model
				// UUID as prefix to ensure uniqueness per model. However since
				// not all providers support subnets, it can be empty, hence both
				// unique and sparse.
				Key:    []string{"providerid"},
				Unique: true,
				Sparse: true,
			}},
		},
		endpointBindingsC: {},

		// -----

		// These collections hold information associated with actions.
		actionsC:             {},
		actionNotificationsC: {},

		// -----

		// TODO(ericsnow) Use a component-oriented registration mechanism...

		// This collection holds information associated with charm payloads.
		// See payload/persistence/mongo.go.
		"payloads": {},

		// This collection holds information associated with charm resources.
		// See resource/persistence/mongo.go.
		"resources": {},

		// -----

		// The remaining non-global collections share the property of being
		// relevant to multiple other kinds of entities, and are thus generally
		// indexed by globalKey(). This is unhelpfully named in this context --
		// it's meant to imply "global within an model", because it was
		// named before multi-env support.

		// This collection holds user annotations for various entities. They
		// shouldn't be written or interpreted by juju.
		annotationsC: {},

		// This collection in particular holds an astounding number of
		// different sorts of data: service config settings by charm version,
		// unit relation settings, model config, etc etc etc.
		settingsC: {},

		constraintsC:        {},
		storageConstraintsC: {},
		statusesC:           {},
		statusesHistoryC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "globalkey"},
			}},
		},

		// This collection holds information about cloud image metadata.
		cloudimagemetadataC: {},

		// ----------------------

		// Raw-access collections
		// ======================

		// metrics; status-history; logs; ..?
	}
}

// These constants are used to avoid sprinkling the package with any more
// magic strings. If a collection deserves documentation, please document
// it in allCollections, above; and please keep this list sorted for easy
// inspection.
const (
	actionNotificationsC     = "actionnotifications"
	actionresultsC           = "actionresults"
	actionsC                 = "actions"
	annotationsC             = "annotations"
	assignUnitC              = "assignUnits"
	blockDevicesC            = "blockdevices"
	blocksC                  = "blocks"
	charmsC                  = "charms"
	cleanupsC                = "cleanups"
	cloudimagemetadataC      = "cloudimagemetadata"
	constraintsC             = "constraints"
	containerRefsC           = "containerRefs"
	controllersC             = "controllers"
	filesystemAttachmentsC   = "filesystemAttachments"
	filesystemsC             = "filesystems"
	instanceDataC            = "instanceData"
	ipaddressesC             = "ipaddresses"
	leaseC                   = "lease"
	leasesC                  = "leases"
	machinesC                = "machines"
	meterStatusC             = "meterStatus"
	metricsC                 = "metrics"
	metricsManagerC          = "metricsmanager"
	minUnitsC                = "minunits"
	modelMigrationStatusC    = "modelmigrations.status"
	modelMigrationsActiveC   = "modelmigrations.active"
	modelMigrationsC         = "modelmigrations"
	modelUserLastConnectionC = "modelUserLastConnection"
	modelUsersC              = "modelusers"
	modelsC                  = "models"
	networkInterfacesC       = "networkinterfaces"
	networksC                = "networks"
	openedPortsC             = "openedPorts"
	rebootC                  = "reboot"
	relationScopesC          = "relationscopes"
	relationsC               = "relations"
	requestedNetworksC       = "requestednetworks"
	restoreInfoC             = "restoreInfo"
	sequenceC                = "sequence"
	servicesC                = "services"
	endpointBindingsC        = "endpointbindings"
	settingsC                = "settings"
	settingsrefsC            = "settingsrefs"
	spacesC                  = "spaces"
	statusesC                = "statuses"
	statusesHistoryC         = "statuseshistory"
	storageAttachmentsC      = "storageattachments"
	storageConstraintsC      = "storageconstraints"
	storageInstancesC        = "storageinstances"
	subnetsC                 = "subnets"
	toolsmetadataC           = "toolsmetadata"
	txnLogC                  = "txns.log"
	txnsC                    = "txns"
	unitsC                   = "units"
	upgradeInfoC             = "upgradeInfo"
	userLastLoginC           = "userLastLogin"
	usermodelnameC           = "usermodelname"
	usersC                   = "users"
	volumeAttachmentsC       = "volumeattachments"
	volumesC                 = "volumes"
	// "payloads" (see payload/persistence/mongo.go)
	// "resources" (see resource/persistence/mongo.go)
)
