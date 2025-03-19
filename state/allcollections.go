// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/mgo/v3"

	"github.com/juju/juju/state/bakerystorage"
	"github.com/juju/juju/state/cloudimagemetadata"
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

		// This collection records the model migrations which
		// are currently in progress. It is used to ensure that only
		// one model migration document exists per model.
		migrationsActiveC: {global: true},

		// This collection tracks migration progress reports from the
		// migration minions.
		migrationsMinionSyncC: {global: true},

		// This collection holds user information that's not specific to any
		// one model.
		usersC: {
			global: true,
		},

		// This collection holds users that are relative to controllers.
		controllerUsersC: {
			global: true,
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

		// This collection holds cloud definitions.
		cloudsC: {global: true},

		// This collection holds users' cloud credentials.
		cloudCredentialsC: {
			global: true,
			indexes: []mgo.Index{{
				Key: []string{"owner", "cloud"},
			}},
		},

		// This collection holds settings from various sources which
		// are inherited and then forked by new models.
		globalSettingsC: {global: true},

		// This collection holds workload metrics reported by certain charms
		// for passing onward to other tools.
		metricsC: {
			global: true,
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "sent"},
			}},
		},

		// This collection holds persistent state for the metrics manager.
		metricsManagerC: {global: true},

		// This collection was deprecated before multi-model support
		// was implemented.
		actionresultsC: {global: true},

		// This collection holds storage items for a macaroon bakery.
		bakeryStorageItemsC: {
			global:  true,
			indexes: bakerystorage.MongoIndexes(),
		},

		// This collection is basically a standard SQL intersection table; it
		// references the global records of the users allowed access to a
		// given operation.
		permissionsC: {
			global: true,
			indexes: []mgo.Index{{
				Key: []string{"object-global-key", "subject-global-key"},
			}},
		},

		// This collection holds information cached by autocert certificate
		// acquisition.
		autocertCacheC: {
			global:    true,
			rawAccess: true,
		},

		// This collection holds the last time the model user connected
		// to the model.
		modelUserLastConnectionC: {
			rawAccess: true,
		},

		// -----------------

		// Local collections
		// =================

		// This collection holds users related to a model and will be used as one
		// of the intersection axis of permissionsC
		modelUsersC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "user"},
			}},
		},

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
		charmsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}, {
				Key: []string{"bundlesha256"},
			}},
		},
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
		minUnitsC: {},

		// This collection holds documents that indicate units which are queued
		// to be assigned to machines. It is used exclusively by the
		// AssignUnitWorker.
		assignUnitC: {},

		// meterStatusC is the collection used to store meter status information.
		meterStatusC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

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

		// Stores Docker image resource details
		dockerResourcesC: {},

		// -----

		// These collections hold information associated with machines.
		containerRefsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},
		instanceDataC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "machineid"},
			}, {
				Key: []string{"model-uuid", "instanceid"},
			}},
		},
		machinesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "machineid"},
			}},
		},
		rebootC: {
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

		// this collection contains machine update locks whose existence indicates
		// that a particular machine in the process of performing a series upgrade.
		machineUpgradeSeriesLocksC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "machineid"},
			}},
		},

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
		spacesC: {
			indexes: []mgo.Index{
				{Key: []string{"model-uuid", "spaceid"}},
				{Key: []string{"model-uuid", "name"}},
			},
		},
		subnetsC: {
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
		openedPortsC: {
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

		// This collection holds information associated with charm payloads.
		payloadsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "unitid"},
			}, {
				Key: []string{"model-uuid", "name"},
			}},
		},

		// This collection holds information associated with charm resources.
		// See resource/persistence/mongo.go, where it should never have
		// been put in the first place.
		"resources": {},
		// see vendor/github.com/juju/blobstore/v2/resourcecatalog.go
		// This shouldn't need to be declared here, but we need to allocate the
		// collection before a TXN tries to insert it.
		"storedResources": {},

		// -----

		// The remaining non-global collections share the property of being
		// relevant to multiple other kinds of entities, and are thus generally
		// indexed by globalKey(). This is unhelpfully named in this context --
		// it's meant to imply "global within an model", because it was
		// named before multi-model support.

		// This collection holds user annotations for various entities. They
		// shouldn't be written or interpreted by juju.
		annotationsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// This collection in particular holds an astounding number of
		// different sorts of data: application config settings by charm version,
		// unit relation settings, model config, etc etc etc.
		settingsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// The generations collection holds data about
		// active and completed "next" model generations.
		generationsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "completed"},
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
		deviceConstraintsC: {},
		statusesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "_id"},
			}},
		},
		statusesHistoryC: {
			rawAccess: true,
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "globalkey", "updated"},
			}, {
				// used for migration and model-specific pruning
				Key: []string{"model-uuid", "-updated", "-_id"},
			}, {
				// used for global pruning (after size check)
				Key: []string{"-updated"},
			}},
		},

		// This collection holds information about cloud image metadata.
		cloudimagemetadataC: {
			global:  true,
			indexes: cloudimagemetadata.MongoIndexes(),
		},

		// Cross model relations collections.
		applicationOffersC: {
			indexes: []mgo.Index{
				{Key: []string{"model-uuid", "_id"}},
				{Key: []string{"model-uuid", "application-name"}},
			},
		},
		offerConnectionsC: {
			indexes: []mgo.Index{
				{Key: []string{"model-uuid", "offer-uuid"}},
				{Key: []string{"model-uuid", "username"}},
			},
		},
		remoteApplicationsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},
		// remoteEntitiesC holds information about entities involved in
		// cross-model relations.
		remoteEntitiesC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "token"},
			}},
		},
		// externalControllersC holds connection information for other
		// controllers hosting models involved in cross-model relations.
		externalControllersC: {
			global: true,
		},
		// relationNetworksC holds required ingress or egress cidrs for remote relations.
		relationNetworksC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// podSpecsC holds the CAAS pod specifications,
		// for applications.
		podSpecsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
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

		secretMetadataC: {
			indexes: []mgo.Index{{
				Key: []string{"owner-tag", "label", "model-uuid"},
			}},
		},

		secretRevisionsC: {
			indexes: []mgo.Index{{
				Key: []string{"revision", "_id"},
			}},
		},

		secretConsumersC: {
			indexes: []mgo.Index{{
				Key: []string{"consumer-tag", "label", "model-uuid"},
			}},
		},

		secretRemoteConsumersC: {
			indexes: []mgo.Index{{
				Key: []string{"consumer-tag", "model-uuid"},
			}},
		},

		secretPermissionsC: {
			indexes: []mgo.Index{{
				Key: []string{"subject-tag", "scope-tag", "model-uuid"},
			}},
		},

		secretRotateC: {
			indexes: []mgo.Index{{
				Key: []string{"owner-tag", "model-uuid"},
			}},
		},

		secretBackendsC: {
			global: true,
			indexes: []mgo.Index{{
				Key: []string{"name"},
			}},
		},

		secretBackendsRotateC: {
			global: true,
			indexes: []mgo.Index{{
				Key: []string{"model-uuid"},
			}},
		},

		// virtualHostKeysC holds host keys that are used
		// for proxying SSH connections through the Juju
		// controller. They are virtual because they do not
		// represent the actual host key on a machine.
		virtualHostKeysC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "_id"},
			}},
		},

		// sshConnRequestsC holds the ssh connection requests.
		// The documents are added/removed by the controller, and units are watching
		// the collection to start a ssh connection to controllers.
		sshConnRequestsC: {
			indexes: []mgo.Index{{
				Key: []string{"model-uuid", "_id"},
			}},
		},

		// ----------------------

		// Raw-access collections
		// ======================

		// metrics; status-history; logs; ..?

	}
	return result
}

// These constants are used to avoid sprinkling the package with any more
// magic strings. If a collection deserves documentation, please document
// it in allCollections, above; and please keep this list sorted for easy
// inspection.
const (
	actionNotificationsC       = "actionnotifications"
	actionresultsC             = "actionresults"
	actionsC                   = "actions"
	annotationsC               = "annotations"
	autocertCacheC             = "autocertCache"
	assignUnitC                = "assignUnits"
	bakeryStorageItemsC        = "bakeryStorageItems"
	blockDevicesC              = "blockdevices"
	blocksC                    = "blocks"
	charmsC                    = "charms"
	cleanupsC                  = "cleanups"
	cloudimagemetadataC        = "cloudimagemetadata"
	cloudsC                    = "clouds"
	cloudContainersC           = "cloudcontainers"
	cloudServicesC             = "cloudservices"
	cloudCredentialsC          = "cloudCredentials"
	constraintsC               = "constraints"
	containerRefsC             = "containerRefs"
	controllersC               = "controllers"
	controllerNodesC           = "controllerNodes"
	controllerUsersC           = "controllerusers"
	dockerResourcesC           = "dockerResources"
	filesystemAttachmentsC     = "filesystemAttachments"
	filesystemsC               = "filesystems"
	globalClockC               = "globalclock"
	globalRefcountsC           = "globalRefcounts"
	globalSettingsC            = "globalSettings"
	instanceDataC              = "instanceData"
	machinesC                  = "machines"
	machineRemovalsC           = "machineremovals"
	machineUpgradeSeriesLocksC = "machineUpgradeSeriesLocks"
	meterStatusC               = "meterStatus"
	metricsC                   = "metrics"
	metricsManagerC            = "metricsmanager"
	minUnitsC                  = "minunits"
	migrationsActiveC          = "migrations.active"
	migrationsC                = "migrations"
	migrationsMinionSyncC      = "migrations.minionsync"
	migrationsStatusC          = "migrations.status"
	modelUserLastConnectionC   = "modelUserLastConnection"
	modelUsersC                = "modelusers"
	modelsC                    = "models"
	modelEntityRefsC           = "modelEntityRefs"
	openedPortsC               = "openedPorts"
	operationsC                = "operations"
	payloadsC                  = "payloads"
	permissionsC               = "permissions"
	podSpecsC                  = "podSpecs"
	providerIDsC               = "providerIDs"
	rebootC                    = "reboot"
	relationScopesC            = "relationscopes"
	relationsC                 = "relations"
	sequenceC                  = "sequence"
	applicationsC              = "applications"
	endpointBindingsC          = "endpointbindings"
	settingsC                  = "settings"
	generationsC               = "generations"
	refcountsC                 = "refcounts"
	resourcesC                 = "resources"
	sshHostKeysC               = "sshhostkeys"
	sshConnRequestsC           = "sshrequests"
	spacesC                    = "spaces"
	statusesC                  = "statuses"
	statusesHistoryC           = "statuseshistory"
	storageAttachmentsC        = "storageattachments"
	storageConstraintsC        = "storageconstraints"
	deviceConstraintsC         = "deviceConstraints"
	storageInstancesC          = "storageinstances"
	subnetsC                   = "subnets"
	linkLayerDevicesC          = "linklayerdevices"
	ipAddressesC               = "ip.addresses"
	toolsmetadataC             = "toolsmetadata"
	txnsC                      = "txns"
	unitsC                     = "units"
	unitStatesC                = "unitstates"
	upgradeInfoC               = "upgradeInfo"
	userLastLoginC             = "userLastLogin"
	usermodelnameC             = "usermodelname"
	usersC                     = "users"
	virtualHostKeysC           = "virtualhostkeys"
	volumeAttachmentsC         = "volumeattachments"
	volumeAttachmentPlanC      = "volumeattachmentplan"
	volumesC                   = "volumes"

	// Cross model relations
	applicationOffersC   = "applicationOffers"
	remoteApplicationsC  = "remoteApplications"
	offerConnectionsC    = "applicationOfferConnections"
	remoteEntitiesC      = "remoteEntities"
	externalControllersC = "externalControllers"
	relationNetworksC    = "relationNetworks"

	// Secrets
	secretMetadataC        = "secretMetadata"
	secretRevisionsC       = "secretRevisions"
	secretConsumersC       = "secretConsumers"
	secretRemoteConsumersC = "secretRemoteConsumers"
	secretPermissionsC     = "secretPermissions"
	secretRotateC          = "secretRotate"
	secretBackendsC        = "secretBackends"
	secretBackendsRotateC  = "secretBackendsRotate"
)

// watcherIgnoreList contains all the collections in mongo that should not be watched by the
// TxnWatcher.
var watcherIgnoreList = []string{
	bakeryStorageItemsC,
	sequenceC,
	refcountsC,
	statusesHistoryC,
}
