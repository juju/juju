// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/action"
	"github.com/juju/juju/apiserver/agent" // ModelUser Write
	"github.com/juju/juju/apiserver/agenttools"
	"github.com/juju/juju/apiserver/annotations" // ModelUser Write
	"github.com/juju/juju/apiserver/application" // ModelUser Write
	"github.com/juju/juju/apiserver/applicationoffers"
	"github.com/juju/juju/apiserver/applicationscaler"
	"github.com/juju/juju/apiserver/backups" // ModelUser Write
	"github.com/juju/juju/apiserver/block"   // ModelUser Write
	"github.com/juju/juju/apiserver/bundle"
	"github.com/juju/juju/apiserver/charmrevisionupdater"
	"github.com/juju/juju/apiserver/charms" // ModelUser Write
	"github.com/juju/juju/apiserver/cleaner"
	"github.com/juju/juju/apiserver/client" // ModelUser Write
	"github.com/juju/juju/apiserver/cloud"  // ModelUser Read
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/controller" // ModelUser Admin (although some methods check for read only)
	"github.com/juju/juju/apiserver/deployer"
	"github.com/juju/juju/apiserver/diskmanager"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/firewaller"
	"github.com/juju/juju/apiserver/highavailability" // ModelUser Write
	"github.com/juju/juju/apiserver/hostkeyreporter"
	"github.com/juju/juju/apiserver/imagemanager" // ModelUser Write
	"github.com/juju/juju/apiserver/imagemetadata"
	"github.com/juju/juju/apiserver/instancepoller"
	"github.com/juju/juju/apiserver/keymanager" // ModelUser Write
	"github.com/juju/juju/apiserver/keyupdater"
	"github.com/juju/juju/apiserver/leadership"
	"github.com/juju/juju/apiserver/lifeflag"
	"github.com/juju/juju/apiserver/logfwd"
	loggerapi "github.com/juju/juju/apiserver/logger"
	"github.com/juju/juju/apiserver/machine"
	"github.com/juju/juju/apiserver/machineactions"
	"github.com/juju/juju/apiserver/machinemanager" // ModelUser Write
	"github.com/juju/juju/apiserver/machineundertaker"
	"github.com/juju/juju/apiserver/meterstatus"
	"github.com/juju/juju/apiserver/metricsadder"
	"github.com/juju/juju/apiserver/metricsdebug" // ModelUser Write
	"github.com/juju/juju/apiserver/metricsmanager"
	"github.com/juju/juju/apiserver/migrationflag"
	"github.com/juju/juju/apiserver/migrationmaster"
	"github.com/juju/juju/apiserver/migrationminion"
	"github.com/juju/juju/apiserver/migrationtarget" // ModelUser Write
	"github.com/juju/juju/apiserver/modelconfig"     // ModelUser Write
	"github.com/juju/juju/apiserver/modelmanager"    // ModelUser Write
	"github.com/juju/juju/apiserver/payloads"
	"github.com/juju/juju/apiserver/payloadshookcontext"
	"github.com/juju/juju/apiserver/provisioner"
	"github.com/juju/juju/apiserver/proxyupdater"
	"github.com/juju/juju/apiserver/reboot"
	"github.com/juju/juju/apiserver/remotefirewaller"
	"github.com/juju/juju/apiserver/remoterelations"
	"github.com/juju/juju/apiserver/resources"
	"github.com/juju/juju/apiserver/resourceshookcontext"
	"github.com/juju/juju/apiserver/resumer"
	"github.com/juju/juju/apiserver/retrystrategy"
	"github.com/juju/juju/apiserver/singular"
	"github.com/juju/juju/apiserver/spaces"    // ModelUser Write
	"github.com/juju/juju/apiserver/sshclient" // ModelUser Write
	"github.com/juju/juju/apiserver/statushistory"
	"github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/apiserver/storageprovisioner"
	"github.com/juju/juju/apiserver/subnets"
	"github.com/juju/juju/apiserver/undertaker"
	"github.com/juju/juju/apiserver/unitassigner"
	"github.com/juju/juju/apiserver/uniter"
	"github.com/juju/juju/apiserver/upgrader"
	"github.com/juju/juju/apiserver/usermanager"
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
	reg("Agent", 2, agent.NewAgentAPIV2)
	reg("AgentTools", 1, agenttools.NewFacade)
	reg("Annotations", 2, annotations.NewAPI)

	reg("Application", 1, application.NewFacade)
	reg("Application", 2, application.NewFacade)
	reg("Application", 3, application.NewFacade)
	reg("Application", 4, application.NewFacade)

	reg("ApplicationScaler", 1, applicationscaler.NewAPI)
	reg("Backups", 1, backups.NewFacade)
	reg("Block", 2, block.NewAPI)
	reg("Bundle", 1, bundle.NewFacade)
	reg("CharmRevisionUpdater", 2, charmrevisionupdater.NewCharmRevisionUpdaterAPI)
	reg("Charms", 2, charms.NewFacade)
	reg("Cleaner", 2, cleaner.NewCleanerAPI)
	reg("Client", 1, client.NewFacade)
	reg("Cloud", 1, cloud.NewFacade)
	reg("Controller", 3, controller.NewControllerAPI)
	reg("Deployer", 1, deployer.NewDeployerAPI)
	reg("DiskManager", 2, diskmanager.NewDiskManagerAPI)
	reg("Firewaller", 3, firewaller.NewFirewallerAPI)
	reg("HighAvailability", 2, highavailability.NewHighAvailabilityAPI)
	reg("HostKeyReporter", 1, hostkeyreporter.NewFacade)
	reg("ImageManager", 2, imagemanager.NewImageManagerAPI)
	reg("ImageMetadata", 2, imagemetadata.NewAPI)
	reg("InstancePoller", 3, instancepoller.NewFacade)
	reg("KeyManager", 1, keymanager.NewKeyManagerAPI)
	reg("KeyUpdater", 1, keyupdater.NewKeyUpdaterAPI)
	reg("LeadershipService", 2, leadership.NewLeadershipServiceFacade)
	reg("LifeFlag", 1, lifeflag.NewExternalFacade)
	reg("Logger", 1, loggerapi.NewLoggerAPI)
	reg("LogForwarding", 1, logfwd.NewFacade)
	reg("MachineActions", 1, machineactions.NewExternalFacade)

	reg("MachineManager", 2, machinemanager.NewMachineManagerAPI)
	reg("MachineManager", 3, machinemanager.NewMachineManagerAPI) // Version 3 adds DestroyMachine and ForceDestroyMachine.

	reg("MachineUndertaker", 1, machineundertaker.NewFacade)
	reg("Machiner", 1, machine.NewMachinerAPI)

	reg("MeterStatus", 1, meterstatus.NewMeterStatusAPI)
	reg("MetricsAdder", 2, metricsadder.NewMetricsAdderAPI)
	reg("MetricsDebug", 2, metricsdebug.NewMetricsDebugAPI)
	reg("MetricsManager", 1, metricsmanager.NewFacade)

	reg("MigrationFlag", 1, migrationflag.NewFacade)
	reg("MigrationMaster", 1, migrationmaster.NewFacade)
	reg("MigrationMinion", 1, migrationminion.NewFacade)
	reg("MigrationTarget", 1, migrationtarget.NewFacade)

	reg("ModelConfig", 1, modelconfig.NewFacade)
	reg("ModelManager", 2, modelmanager.NewFacadeV2)
	reg("ModelManager", 3, modelmanager.NewFacadeV3)

	reg("Payloads", 1, payloads.NewFacade)
	regHookContext(
		"PayloadsHookContext", 1,
		payloadshookcontext.NewHookContextFacade,
		reflect.TypeOf(&payloadshookcontext.UnitFacade{}),
	)

	reg("Pinger", 1, NewPinger)
	reg("Provisioner", 3, provisioner.NewProvisionerAPI)
	reg("ProxyUpdater", 1, proxyupdater.NewAPI)
	reg("Reboot", 2, reboot.NewRebootAPI)

	reg("Resources", 1, resources.NewPublicFacade)
	regHookContext(
		"ResourcesHookContext", 1,
		resourceshookcontext.NewHookContextFacade,
		reflect.TypeOf(&resourceshookcontext.UnitFacade{}),
	)

	reg("Resumer", 2, resumer.NewResumerAPI)
	reg("RetryStrategy", 1, retrystrategy.NewRetryStrategyAPI)
	reg("Singular", 1, singular.NewExternalFacade)

	reg("SSHClient", 1, sshclient.NewFacade)
	reg("SSHClient", 2, sshclient.NewFacade) // v2 adds AllAddresses() method.

	reg("Spaces", 2, spaces.NewAPIV2)
	reg("Spaces", 3, spaces.NewAPI)

	reg("StatusHistory", 2, statushistory.NewAPI)
	reg("Storage", 3, storage.NewFacade)
	reg("StorageProvisioner", 3, storageprovisioner.NewFacade)
	reg("Subnets", 2, subnets.NewAPI)
	reg("Undertaker", 1, undertaker.NewUndertakerAPI)
	reg("UnitAssigner", 1, unitassigner.New)

	reg("Uniter", 4, uniter.NewUniterAPIV4)
	reg("Uniter", 5, uniter.NewUniterAPIV5)
	reg("Uniter", 6, uniter.NewUniterAPI)

	reg("Upgrader", 1, upgrader.NewUpgraderFacade)
	reg("UserManager", 1, usermanager.NewUserManagerAPI)

	if featureflag.Enabled(feature.CrossModelRelations) {
		reg("ApplicationOffers", 1, applicationoffers.NewOffersAPI)
		reg("RemoteFirewaller", 1, remotefirewaller.NewStateRemoteFirewallerAPI)
		reg("RemoteRelations", 1, remoterelations.NewStateRemoteRelationsAPI)
	}

	regRaw("AllWatcher", 1, NewAllWatcher, reflect.TypeOf((*SrvAllWatcher)(nil)))
	// Note: AllModelWatcher uses the same infrastructure as AllWatcher
	// but they are get under separate names as it possible the may
	// diverge in the future (especially in terms of authorisation
	// checks).
	regRaw("AllModelWatcher", 2, NewAllWatcher, reflect.TypeOf((*SrvAllWatcher)(nil)))
	regRaw("NotifyWatcher", 1, newNotifyWatcher, reflect.TypeOf((*srvNotifyWatcher)(nil)))
	regRaw("StringsWatcher", 1, newStringsWatcher, reflect.TypeOf((*srvStringsWatcher)(nil)))
	regRaw("RelationUnitsWatcher", 1, newRelationUnitsWatcher, reflect.TypeOf((*srvRelationUnitsWatcher)(nil)))
	regRaw("VolumeAttachmentsWatcher", 2, newVolumeAttachmentsWatcher, reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)))
	regRaw("FilesystemAttachmentsWatcher", 2, newFilesystemAttachmentsWatcher, reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)))
	regRaw("EntityWatcher", 2, newEntitiesWatcher, reflect.TypeOf((*srvEntitiesWatcher)(nil)))
	regRaw("MigrationStatusWatcher", 1, newMigrationStatusWatcher, reflect.TypeOf((*srvMigrationStatusWatcher)(nil)))

	return registry
}

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
