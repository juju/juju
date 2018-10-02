// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/agent" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/agent/caasagent"
	"github.com/juju/juju/apiserver/facades/agent/caasoperator"
	"github.com/juju/juju/apiserver/facades/agent/credentialvalidator"
	"github.com/juju/juju/apiserver/facades/agent/deployer"
	"github.com/juju/juju/apiserver/facades/agent/diskmanager"
	"github.com/juju/juju/apiserver/facades/agent/fanconfigurer"
	"github.com/juju/juju/apiserver/facades/agent/hostkeyreporter"
	"github.com/juju/juju/apiserver/facades/agent/keyupdater"
	"github.com/juju/juju/apiserver/facades/agent/leadership"
	loggerapi "github.com/juju/juju/apiserver/facades/agent/logger"
	"github.com/juju/juju/apiserver/facades/agent/machine"
	"github.com/juju/juju/apiserver/facades/agent/machineactions"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
	"github.com/juju/juju/apiserver/facades/agent/metricsadder"
	"github.com/juju/juju/apiserver/facades/agent/migrationflag"
	"github.com/juju/juju/apiserver/facades/agent/migrationminion"
	"github.com/juju/juju/apiserver/facades/agent/payloadshookcontext"
	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	"github.com/juju/juju/apiserver/facades/agent/proxyupdater"
	"github.com/juju/juju/apiserver/facades/agent/reboot"
	"github.com/juju/juju/apiserver/facades/agent/resourceshookcontext"
	"github.com/juju/juju/apiserver/facades/agent/retrystrategy"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	"github.com/juju/juju/apiserver/facades/agent/unitassigner"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/facades/agent/upgrader"
	"github.com/juju/juju/apiserver/facades/agent/upgradeseries"
	"github.com/juju/juju/apiserver/facades/client/action"
	"github.com/juju/juju/apiserver/facades/client/annotations" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/application" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/applicationoffers"
	"github.com/juju/juju/apiserver/facades/client/backups" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/block"   // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/apiserver/facades/client/charms"     // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/client"     // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/cloud"      // ModelUser Read
	"github.com/juju/juju/apiserver/facades/client/controller" // ModelUser Admin (although some methods check for read only)
	"github.com/juju/juju/apiserver/facades/client/credentialmanager"
	"github.com/juju/juju/apiserver/facades/client/firewallrules"
	"github.com/juju/juju/apiserver/facades/client/highavailability" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/imagemanager"     // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/imagemetadatamanager"
	"github.com/juju/juju/apiserver/facades/client/keymanager"                  // ModelUser Write
	leadershipclient "github.com/juju/juju/apiserver/facades/client/leadership" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/machinemanager"              // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/metricsdebug"                // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/modelconfig"                 // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/modelmanager"                // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/payloads"
	"github.com/juju/juju/apiserver/facades/client/resources"
	"github.com/juju/juju/apiserver/facades/client/spaces"    // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/sshclient" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/storage"
	"github.com/juju/juju/apiserver/facades/client/subnets"
	"github.com/juju/juju/apiserver/facades/client/usermanager"
	"github.com/juju/juju/apiserver/facades/controller/actionpruner"
	"github.com/juju/juju/apiserver/facades/controller/agenttools"
	"github.com/juju/juju/apiserver/facades/controller/applicationscaler"
	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/apiserver/facades/controller/cleaner"
	"github.com/juju/juju/apiserver/facades/controller/crosscontroller"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater"
	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/apiserver/facades/controller/imagemetadata"
	"github.com/juju/juju/apiserver/facades/controller/instancepoller"
	"github.com/juju/juju/apiserver/facades/controller/lifeflag"
	"github.com/juju/juju/apiserver/facades/controller/logfwd"
	"github.com/juju/juju/apiserver/facades/controller/machineundertaker"
	"github.com/juju/juju/apiserver/facades/controller/metricsmanager"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/controller/modelupgrader"
	"github.com/juju/juju/apiserver/facades/controller/remoterelations"
	"github.com/juju/juju/apiserver/facades/controller/resumer"
	"github.com/juju/juju/apiserver/facades/controller/singular"
	"github.com/juju/juju/apiserver/facades/controller/statushistory"
	"github.com/juju/juju/apiserver/facades/controller/undertaker"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

// AllFacades returns a registry containing all known API facades.
//
// This will panic if facade registration fails, but there is a unit
// test to guard against that.
func AllFacades() *facade.Registry {
	registry := new(facade.Registry)

	reg := func(name string, version int, newFunc interface{}) {
		err := registry.RegisterStandard(name, version, newFunc)
		if err != nil {
			panic(err)
		}
	}

	regRaw := func(name string, version int, factory facade.Factory, facadeType reflect.Type) {
		err := registry.Register(name, version, factory, facadeType)
		if err != nil {
			panic(err)
		}
	}

	regHookContext := func(name string, version int, newHookContextFacade hookContextFacadeFn, facadeType reflect.Type) {
		err := regHookContextFacade(registry, name, version, newHookContextFacade, facadeType)
		if err != nil {
			panic(err)
		}
	}

	reg("Action", 2, action.NewActionAPI)
	reg("ActionPruner", 1, actionpruner.NewAPI)
	reg("Agent", 2, agent.NewAgentAPIV2)
	reg("AgentTools", 1, agenttools.NewFacade)
	reg("Annotations", 2, annotations.NewAPI)

	// Application facade versions 1-4 share NewFacadeV4 as
	// the newer methodology for versioning wasn't started with
	// Application until version 5.
	reg("Application", 1, application.NewFacadeV4)
	reg("Application", 2, application.NewFacadeV4)
	reg("Application", 3, application.NewFacadeV4)
	reg("Application", 4, application.NewFacadeV4)
	reg("Application", 5, application.NewFacadeV5) // adds AttachStorage & UpdateApplicationSeries & SetRelationStatus
	reg("Application", 6, application.NewFacadeV6)
	reg("Application", 7, application.NewFacadeV7)
	reg("Application", 8, application.NewFacadeV8)

	reg("ApplicationOffers", 1, applicationoffers.NewOffersAPI)
	reg("ApplicationOffers", 2, applicationoffers.NewOffersAPIV2)
	reg("ApplicationScaler", 1, applicationscaler.NewAPI)
	reg("Backups", 1, backups.NewFacade)
	reg("Backups", 2, backups.NewFacadeV2)
	reg("Block", 2, block.NewAPI)
	reg("Bundle", 1, bundle.NewFacadeV1)
	reg("Bundle", 2, bundle.NewFacadeV2)
	reg("CharmRevisionUpdater", 2, charmrevisionupdater.NewCharmRevisionUpdaterAPI)
	reg("Charms", 2, charms.NewFacade)
	reg("Cleaner", 2, cleaner.NewCleanerAPI)
	reg("Client", 1, client.NewFacadeV1)
	reg("Client", 2, client.NewFacade)
	reg("Cloud", 1, cloud.NewFacadeV1)
	reg("Cloud", 2, cloud.NewFacadeV2) // adds AddCloud, AddCredentials, CredentialContents, RemoveClouds
	reg("Cloud", 3, cloud.NewFacadeV3) // changes signature of UpdateCredentials, adds ModifyCloudAccess

	// CAAS related facades.
	// Move these to the correct place above once the feature flag disappears.
	reg("CAASFirewaller", 1, caasfirewaller.NewStateFacade)
	reg("CAASOperator", 1, caasoperator.NewStateFacade)
	reg("CAASAgent", 1, caasagent.NewStateFacade)
	reg("CAASOperatorProvisioner", 1, caasoperatorprovisioner.NewStateCAASOperatorProvisionerAPI)
	reg("CAASUnitProvisioner", 1, caasunitprovisioner.NewStateFacade)

	reg("Controller", 3, controller.NewControllerAPIv3)
	reg("Controller", 4, controller.NewControllerAPIv4)
	reg("Controller", 5, controller.NewControllerAPIv5)
	reg("CrossModelRelations", 1, crossmodelrelations.NewStateCrossModelRelationsAPI)
	reg("CrossController", 1, crosscontroller.NewStateCrossControllerAPI)
	reg("CredentialManager", 1, credentialmanager.NewCredentialManagerAPI)
	reg("CredentialValidator", 1, credentialvalidator.NewCredentialValidatorAPI)
	reg("ExternalControllerUpdater", 1, externalcontrollerupdater.NewStateAPI)

	reg("Deployer", 1, deployer.NewDeployerAPI)
	reg("DiskManager", 2, diskmanager.NewDiskManagerAPI)
	reg("FanConfigurer", 1, fanconfigurer.NewFanConfigurerAPI)
	reg("Firewaller", 3, firewaller.NewStateFirewallerAPIV3)
	reg("Firewaller", 4, firewaller.NewStateFirewallerAPIV4)
	reg("Firewaller", 5, firewaller.NewStateFirewallerAPIV5)
	reg("FirewallRules", 1, firewallrules.NewFacade)
	reg("HighAvailability", 2, highavailability.NewHighAvailabilityAPI)
	reg("HostKeyReporter", 1, hostkeyreporter.NewFacade)
	reg("ImageManager", 2, imagemanager.NewImageManagerAPI)
	reg("ImageMetadata", 3, imagemetadata.NewAPI)

	if featureflag.Enabled(feature.ImageMetadata) {
		reg("ImageMetadataManager", 1, imagemetadatamanager.NewAPI)
	}

	reg("InstancePoller", 3, instancepoller.NewFacade)
	reg("KeyManager", 1, keymanager.NewKeyManagerAPI)
	reg("KeyUpdater", 1, keyupdater.NewKeyUpdaterAPI)

	// LeadershipClient is the client API server,
	// whereas LeadershipService is for agents.
	reg("LeadershipClient", 1, leadershipclient.NewFacade)
	reg("LeadershipService", 2, leadership.NewLeadershipServiceFacade)

	reg("LifeFlag", 1, lifeflag.NewExternalFacade)
	reg("Logger", 1, loggerapi.NewLoggerAPI)
	reg("LogForwarding", 1, logfwd.NewFacade)
	reg("MachineActions", 1, machineactions.NewExternalFacade)

	reg("MachineManager", 2, machinemanager.NewFacade)
	reg("MachineManager", 3, machinemanager.NewFacade)   // Version 3 adds DestroyMachine and ForceDestroyMachine.
	reg("MachineManager", 4, machinemanager.NewFacadeV4) // Version 4 adds DestroyMachineWithParams.
	reg("MachineManager", 5, machinemanager.NewFacadeV5) // Version 5 adds UpgradeSeriesPrepare.

	reg("MachineUndertaker", 1, machineundertaker.NewFacade)
	reg("Machiner", 1, machine.NewMachinerAPI)

	reg("MeterStatus", 1, meterstatus.NewMeterStatusFacade)
	reg("MetricsAdder", 2, metricsadder.NewMetricsAdderAPI)
	reg("MetricsDebug", 2, metricsdebug.NewMetricsDebugAPI)
	reg("MetricsManager", 1, metricsmanager.NewFacade)

	reg("MigrationFlag", 1, migrationflag.NewFacade)
	reg("MigrationMaster", 1, migrationmaster.NewFacade)
	reg("MigrationMinion", 1, migrationminion.NewFacade)
	reg("MigrationTarget", 1, migrationtarget.NewFacade)

	reg("ModelConfig", 1, modelconfig.NewFacadeV1)
	reg("ModelConfig", 2, modelconfig.NewFacadeV2)
	reg("ModelManager", 2, modelmanager.NewFacadeV2)
	reg("ModelManager", 3, modelmanager.NewFacadeV3)
	reg("ModelManager", 4, modelmanager.NewFacadeV4)
	reg("ModelManager", 5, modelmanager.NewFacadeV5) // adds ChangeModelCredential
	reg("ModelUpgrader", 1, modelupgrader.NewStateFacade)

	reg("Payloads", 1, payloads.NewFacade)
	regHookContext(
		"PayloadsHookContext", 1,
		payloadshookcontext.NewHookContextFacade,
		reflect.TypeOf(&payloadshookcontext.UnitFacade{}),
	)

	reg("Pinger", 1, NewPinger)
	reg("Provisioner", 3, provisioner.NewProvisionerAPIV4) // Yes this is weird.
	reg("Provisioner", 4, provisioner.NewProvisionerAPIV4)
	reg("Provisioner", 5, provisioner.NewProvisionerAPIV5) // v5 adds DistributionGroupByMachineId()
	reg("Provisioner", 6, provisioner.NewProvisionerAPIV6) // v6 adds more proxy settings
	reg("Provisioner", 7, provisioner.NewProvisionerAPIV7) // v7 adds charm profile watcher

	reg("ProxyUpdater", 1, proxyupdater.NewFacadeV1)
	reg("ProxyUpdater", 2, proxyupdater.NewFacadeV2)
	reg("Reboot", 2, reboot.NewRebootAPI)
	reg("RemoteRelations", 1, remoterelations.NewStateRemoteRelationsAPI)

	reg("Resources", 1, resources.NewPublicFacade)
	reg("ResourcesHookContext", 1, resourceshookcontext.NewStateFacade)

	reg("Resumer", 2, resumer.NewResumerAPI)
	reg("RetryStrategy", 1, retrystrategy.NewRetryStrategyAPI)
	reg("Singular", 2, singular.NewExternalFacade)

	reg("SSHClient", 1, sshclient.NewFacade)
	reg("SSHClient", 2, sshclient.NewFacade) // v2 adds AllAddresses() method.

	reg("Spaces", 2, spaces.NewAPIV2)
	reg("Spaces", 3, spaces.NewAPI)

	reg("StatusHistory", 2, statushistory.NewAPI)

	reg("Storage", 3, storage.NewFacadeV3)
	reg("Storage", 4, storage.NewFacadeV4) // changes Destroy() method signature.

	reg("StorageProvisioner", 3, storageprovisioner.NewFacadeV3)
	reg("StorageProvisioner", 4, storageprovisioner.NewFacadeV4)
	reg("Subnets", 2, subnets.NewAPI)
	reg("Undertaker", 1, undertaker.NewUndertakerAPI)
	reg("UnitAssigner", 1, unitassigner.New)

	reg("Uniter", 4, uniter.NewUniterAPIV4)
	reg("Uniter", 5, uniter.NewUniterAPIV5)
	reg("Uniter", 6, uniter.NewUniterAPIV6)
	reg("Uniter", 7, uniter.NewUniterAPIV7)
	reg("Uniter", 8, uniter.NewUniterAPI)

	reg("Upgrader", 1, upgrader.NewUpgraderFacade)
	reg("UpgradeSeries", 1, upgradeseries.NewAPI)
	reg("UserManager", 1, usermanager.NewUserManagerAPI)
	reg("UserManager", 2, usermanager.NewUserManagerAPI) // Adds ResetPassword

	regRaw("AllWatcher", 1, NewAllWatcher, reflect.TypeOf((*SrvAllWatcher)(nil)))
	// Note: AllModelWatcher uses the same infrastructure as AllWatcher
	// but they are get under separate names as it possible the may
	// diverge in the future (especially in terms of authorisation
	// checks).
	regRaw("AllModelWatcher", 2, NewAllWatcher, reflect.TypeOf((*SrvAllWatcher)(nil)))
	regRaw("NotifyWatcher", 1, newNotifyWatcher, reflect.TypeOf((*srvNotifyWatcher)(nil)))
	regRaw("StringsWatcher", 1, newStringsWatcher, reflect.TypeOf((*srvStringsWatcher)(nil)))
	regRaw("OfferStatusWatcher", 1, newOfferStatusWatcher, reflect.TypeOf((*srvOfferStatusWatcher)(nil)))
	regRaw("RelationStatusWatcher", 1, newRelationStatusWatcher, reflect.TypeOf((*srvRelationStatusWatcher)(nil)))
	regRaw("RelationUnitsWatcher", 1, newRelationUnitsWatcher, reflect.TypeOf((*srvRelationUnitsWatcher)(nil)))
	regRaw("VolumeAttachmentsWatcher", 2, newVolumeAttachmentsWatcher, reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)))
	regRaw("VolumeAttachmentPlansWatcher", 1, newVolumeAttachmentPlansWatcher, reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)))
	regRaw("FilesystemAttachmentsWatcher", 2, newFilesystemAttachmentsWatcher, reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)))
	regRaw("EntityWatcher", 2, newEntitiesWatcher, reflect.TypeOf((*srvEntitiesWatcher)(nil)))
	regRaw("MigrationStatusWatcher", 1, newMigrationStatusWatcher, reflect.TypeOf((*srvMigrationStatusWatcher)(nil)))

	return registry
}

// adminAPIFactories holds methods used to create
// admin APIs with specific versions.
var adminAPIFactories = map[int]adminAPIFactory{
	3: newAdminAPIV3,
}

// AdminFacadeDetails returns information on the Admin facade provided
// at login time. The Facade field of the returned slice elements will
// be nil.
func AdminFacadeDetails() []facade.Details {
	var fs []facade.Details
	for v, f := range adminAPIFactories {
		api := f(nil, nil, nil)
		t := reflect.TypeOf(api)
		fs = append(fs, facade.Details{
			Name:    "Admin",
			Version: v,
			Type:    t,
		})
	}
	return fs
}

type hookContextFacadeFn func(*state.State, *state.Unit) (interface{}, error)

// regHookContextFacade registers facades for use within a hook
// context. This function handles the translation from a
// hook-context-facade to a standard facade so the caller's factory
// method can elide unnecessary arguments. This function also handles
// any necessary authorization for the client.
//
// XXX(fwereade): this is fundamentally broken, because it (1)
// arbitrarily creates a new facade for a tiny fragment of a specific
// client worker's reponsibilities and (2) actively conceals necessary
// auth information from the facade. Don't call it; actively work to
// delete code that uses it, and rewrite it properly.
func regHookContextFacade(
	reg *facade.Registry,
	name string,
	version int,
	newHookContextFacade hookContextFacadeFn,
	facadeType reflect.Type,
) error {
	newFacade := func(context facade.Context) (facade.Facade, error) {
		authorizer := context.Auth()
		st := context.State()

		if !authorizer.AuthUnitAgent() {
			return nil, common.ErrPerm
		}
		// Verify that the unit's ID matches a unit that we know about.
		tag := authorizer.GetAuthTag()
		if _, ok := tag.(names.UnitTag); !ok {
			return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
		}
		unit, err := st.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newHookContextFacade(st, unit)
	}
	err := reg.Register(name, version, newFacade, facadeType)
	return errors.Trace(err)
}
