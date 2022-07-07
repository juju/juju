// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/agent"
	"github.com/juju/juju/apiserver/facades/agent/caasadmission"
	"github.com/juju/juju/apiserver/facades/agent/caasagent"
	"github.com/juju/juju/apiserver/facades/agent/caasapplication"
	"github.com/juju/juju/apiserver/facades/agent/caasoperator"
	"github.com/juju/juju/apiserver/facades/agent/credentialvalidator"
	"github.com/juju/juju/apiserver/facades/agent/deployer"
	"github.com/juju/juju/apiserver/facades/agent/diskmanager"
	"github.com/juju/juju/apiserver/facades/agent/fanconfigurer"
	"github.com/juju/juju/apiserver/facades/agent/hostkeyreporter"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater"
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
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	"github.com/juju/juju/apiserver/facades/agent/unitassigner"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/facades/agent/upgrader"
	"github.com/juju/juju/apiserver/facades/agent/upgradeseries"
	"github.com/juju/juju/apiserver/facades/agent/upgradesteps"
	"github.com/juju/juju/apiserver/facades/client/action"
	"github.com/juju/juju/apiserver/facades/client/annotations" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/modelupgrader"

	// ModelUser Write
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
	"github.com/juju/juju/apiserver/facades/client/keymanager"     // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/machinemanager" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/metricsdebug"   // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/modelconfig"    // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/modelgeneration"
	"github.com/juju/juju/apiserver/facades/client/modelmanager" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/payloads"
	"github.com/juju/juju/apiserver/facades/client/resources"
	"github.com/juju/juju/apiserver/facades/client/secrets"
	"github.com/juju/juju/apiserver/facades/client/spaces"    // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/sshclient" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/storage"
	"github.com/juju/juju/apiserver/facades/client/subnets"
	"github.com/juju/juju/apiserver/facades/client/usermanager"
	"github.com/juju/juju/apiserver/facades/controller/actionpruner"
	"github.com/juju/juju/apiserver/facades/controller/agenttools"
	"github.com/juju/juju/apiserver/facades/controller/applicationscaler"
	"github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner"
	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager"
	"github.com/juju/juju/apiserver/facades/controller/caasmodeloperator"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorupgrader"
	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	"github.com/juju/juju/apiserver/facades/controller/charmdownloader"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/apiserver/facades/controller/cleaner"
	"github.com/juju/juju/apiserver/facades/controller/crosscontroller"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/apiserver/facades/controller/environupgrader"
	"github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater"
	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/apiserver/facades/controller/imagemetadata"
	"github.com/juju/juju/apiserver/facades/controller/instancepoller"
	"github.com/juju/juju/apiserver/facades/controller/lifeflag"
	"github.com/juju/juju/apiserver/facades/controller/logfwd"
	"github.com/juju/juju/apiserver/facades/controller/machineundertaker"
	"github.com/juju/juju/apiserver/facades/controller/metricsmanager"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	"github.com/juju/juju/apiserver/facades/controller/raftlease"
	"github.com/juju/juju/apiserver/facades/controller/remoterelations"
	"github.com/juju/juju/apiserver/facades/controller/resumer"
	"github.com/juju/juju/apiserver/facades/controller/singular"
	"github.com/juju/juju/apiserver/facades/controller/statushistory"
	"github.com/juju/juju/apiserver/facades/controller/undertaker"
)

// AllFacades returns a registry containing all known API facades.
//
// This will panic if facade registration fails, but there is a unit
// test to guard against that.
func AllFacades() *facade.Registry {
	registry := new(facade.Registry)

	action.Register(registry)
	actionpruner.Register(registry)
	agent.Register(registry)
	agenttools.Register(registry)
	annotations.Register(registry)
	application.Register(registry)
	applicationoffers.Register(registry)
	applicationscaler.Register(registry)
	backups.Register(registry)
	block.Register(registry)
	bundle.Register(registry)
	charmdownloader.Register(registry)
	charmrevisionupdater.Register(registry)
	charms.Register(registry)
	cleaner.Register(registry)
	client.Register(registry)
	cloud.Register(registry)

	// CAAS related facades.
	caasadmission.Register(registry)
	caasagent.Register(registry)
	caasapplication.Register(registry)
	caasapplicationprovisioner.Register(registry)
	caasfirewaller.Register(registry)
	caasoperator.Register(registry)
	caasmodeloperator.Register(registry)
	caasmodelconfigmanager.Register(registry)
	caasoperatorprovisioner.Register(registry)
	caasoperatorupgrader.Register(registry)
	caasunitprovisioner.Register(registry)

	controller.Register(registry)
	crossmodelrelations.Register(registry)
	crosscontroller.Register(registry)
	credentialmanager.Register(registry)
	credentialvalidator.Register(registry)
	externalcontrollerupdater.Register(registry)
	deployer.Register(registry)
	diskmanager.Register(registry)
	environupgrader.Register(registry)
	fanconfigurer.Register(registry)
	firewaller.Register(registry)
	firewallrules.Register(registry)
	highavailability.Register(registry)
	hostkeyreporter.Register(registry)
	imagemanager.Register(registry)
	imagemetadata.Register(registry)
	imagemetadatamanager.Register(registry)
	instancemutater.Register(registry)
	instancepoller.Register(registry)
	keymanager.Register(registry)
	keyupdater.Register(registry)
	leadership.Register(registry)
	lifeflag.Register(registry)
	loggerapi.Register(registry)
	logfwd.Register(registry)
	machineactions.Register(registry)
	machinemanager.Register(registry)
	machineundertaker.Register(registry)
	machine.Register(registry)
	meterstatus.Register(registry)
	metricsadder.Register(registry)
	metricsdebug.Register(registry)
	metricsmanager.Register(registry)
	migrationflag.Register(registry)
	migrationmaster.Register(registry)
	migrationminion.Register(registry)
	migrationtarget.Register(registry)
	modelconfig.Register(registry)
	modelgeneration.Register(registry)
	modelmanager.Register(registry)
	modelupgrader.Register(registry)
	payloads.Register(registry)
	payloadshookcontext.Register(registry)
	provisioner.Register(registry)
	proxyupdater.Register(registry)
	raftlease.Register(registry)
	reboot.Register(registry)
	remoterelations.Register(registry)
	resources.Register(registry)
	resourceshookcontext.Register(registry)
	resumer.Register(registry)
	retrystrategy.Register(registry)
	singular.Register(registry)
	secrets.Register(registry)
	secretsmanager.Register(registry)
	sshclient.Register(registry)
	spaces.Register(registry)
	statushistory.Register(registry)
	storage.Register(registry)
	storageprovisioner.Register(registry)
	subnets.Register(registry)
	undertaker.Register(registry)
	unitassigner.Register(registry)
	uniter.Register(registry)
	upgrader.Register(registry)
	upgradeseries.Register(registry)
	upgradesteps.Register(registry)
	usermanager.Register(registry)

	// TODO (stickupkid): The following should be moved into a package.
	registry.MustRegister("Pinger", 1, func(ctx facade.Context) (facade.Facade, error) {
		return NewPinger(ctx)
	}, reflect.TypeOf((*Pinger)(nil)).Elem())

	registry.MustRegister("AllWatcher", 1, NewAllWatcherV1, reflect.TypeOf((*SrvAllWatcherV1)(nil)))
	registry.MustRegister("AllWatcher", 2, NewAllWatcher, reflect.TypeOf((*SrvAllWatcher)(nil)))
	// Note: AllModelWatcher uses the same infrastructure as AllWatcher
	// but they are get under separate names as it possible the may
	// diverge in the future (especially in terms of authorisation
	// checks).
	registry.MustRegister("AllModelWatcher", 2, NewAllWatcherV1, reflect.TypeOf((*SrvAllWatcherV1)(nil)))
	registry.MustRegister("AllModelWatcher", 3, NewAllWatcher, reflect.TypeOf((*SrvAllWatcher)(nil)))
	registry.MustRegister("NotifyWatcher", 1, newNotifyWatcher, reflect.TypeOf((*srvNotifyWatcher)(nil)))
	registry.MustRegister("StringsWatcher", 1, newStringsWatcher, reflect.TypeOf((*srvStringsWatcher)(nil)))
	registry.MustRegister("OfferStatusWatcher", 1, newOfferStatusWatcher, reflect.TypeOf((*srvOfferStatusWatcher)(nil)))
	registry.MustRegister("RelationStatusWatcher", 1, newRelationStatusWatcher, reflect.TypeOf((*srvRelationStatusWatcher)(nil)))
	registry.MustRegister("RelationUnitsWatcher", 1, newRelationUnitsWatcher, reflect.TypeOf((*srvRelationUnitsWatcher)(nil)))
	registry.MustRegister("RemoteRelationWatcher", 1, newRemoteRelationWatcher, reflect.TypeOf((*srvRemoteRelationWatcher)(nil)))
	registry.MustRegister("VolumeAttachmentsWatcher", 2, newVolumeAttachmentsWatcher, reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)))
	registry.MustRegister("VolumeAttachmentPlansWatcher", 1, newVolumeAttachmentPlansWatcher, reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)))
	registry.MustRegister("FilesystemAttachmentsWatcher", 2, newFilesystemAttachmentsWatcher, reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)))
	registry.MustRegister("EntityWatcher", 2, newEntitiesWatcher, reflect.TypeOf((*srvEntitiesWatcher)(nil)))
	registry.MustRegister("MigrationStatusWatcher", 1, newMigrationStatusWatcher, reflect.TypeOf((*srvMigrationStatusWatcher)(nil)))
	registry.MustRegister("ModelSummaryWatcher", 1, newModelSummaryWatcher, reflect.TypeOf((*SrvModelSummaryWatcher)(nil)))
	registry.MustRegister("SecretsRotationWatcher", 1, newSecretsRotationWatcher, reflect.TypeOf((*srvSecretRotationWatcher)(nil)))

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
