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
	"github.com/juju/juju/apiserver/facades/agent/credentialvalidator"
	"github.com/juju/juju/apiserver/facades/agent/deployer"
	"github.com/juju/juju/apiserver/facades/agent/diskmanager"
	"github.com/juju/juju/apiserver/facades/agent/hostkeyreporter"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater"
	"github.com/juju/juju/apiserver/facades/agent/keyupdater"
	"github.com/juju/juju/apiserver/facades/agent/leadership"
	agentlifeflag "github.com/juju/juju/apiserver/facades/agent/lifeflag"
	loggerapi "github.com/juju/juju/apiserver/facades/agent/logger"
	"github.com/juju/juju/apiserver/facades/agent/machine"
	"github.com/juju/juju/apiserver/facades/agent/machineactions"
	"github.com/juju/juju/apiserver/facades/agent/migrationflag"
	"github.com/juju/juju/apiserver/facades/agent/migrationminion"
	"github.com/juju/juju/apiserver/facades/agent/payloadshookcontext"
	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	"github.com/juju/juju/apiserver/facades/agent/proxyupdater"
	"github.com/juju/juju/apiserver/facades/agent/reboot"
	"github.com/juju/juju/apiserver/facades/agent/resourceshookcontext"
	"github.com/juju/juju/apiserver/facades/agent/retrystrategy"
	"github.com/juju/juju/apiserver/facades/agent/secretsdrain"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	"github.com/juju/juju/apiserver/facades/agent/unitassigner"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/facades/agent/upgrader"
	"github.com/juju/juju/apiserver/facades/client/action"
	"github.com/juju/juju/apiserver/facades/client/annotations" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/applicationoffers" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/backups"           // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/block"             // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/apiserver/facades/client/charms"     // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/client"     // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/cloud"      // ModelUser Read
	"github.com/juju/juju/apiserver/facades/client/controller" // ModelUser Admin (although some methods check for read only)
	"github.com/juju/juju/apiserver/facades/client/credentialmanager"
	"github.com/juju/juju/apiserver/facades/client/highavailability" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/imagemetadatamanager"
	"github.com/juju/juju/apiserver/facades/client/keymanager"     // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/machinemanager" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/modelconfig"    // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/modelmanager"   // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/modelupgrader"
	"github.com/juju/juju/apiserver/facades/client/pinger"
	"github.com/juju/juju/apiserver/facades/client/resources"
	"github.com/juju/juju/apiserver/facades/client/secretbackends"
	"github.com/juju/juju/apiserver/facades/client/secrets"
	"github.com/juju/juju/apiserver/facades/client/spaces"    // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/sshclient" // ModelUser Write
	"github.com/juju/juju/apiserver/facades/client/storage"
	"github.com/juju/juju/apiserver/facades/client/subnets"
	"github.com/juju/juju/apiserver/facades/client/usermanager"
	"github.com/juju/juju/apiserver/facades/controller/agenttools"
	"github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner"
	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager"
	"github.com/juju/juju/apiserver/facades/controller/caasmodeloperator"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorupgrader"
	"github.com/juju/juju/apiserver/facades/controller/cleaner"
	"github.com/juju/juju/apiserver/facades/controller/crosscontroller"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets"
	"github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater"
	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/apiserver/facades/controller/imagemetadata"
	"github.com/juju/juju/apiserver/facades/controller/machineundertaker"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	"github.com/juju/juju/apiserver/facades/controller/remoterelations"
	"github.com/juju/juju/apiserver/facades/controller/secretbackendmanager"
	"github.com/juju/juju/apiserver/facades/controller/undertaker"
	"github.com/juju/juju/apiserver/facades/controller/usersecrets"
	"github.com/juju/juju/apiserver/facades/controller/usersecretsdrain"
	"github.com/juju/juju/core/facades"
)

// requiredMigrationFacadeVersions returns the facade versions that
// must be available for the migration master to function.
// This is a separate function so that it can be used in the
// migrationmaster facade registration as a dependency.
//
// A lot of the agent facades aren't actually required, but they are
// included here to keep the agent alive during migration.
func requiredMigrationFacadeVersions() facades.FacadeVersions {
	registry := new(facade.Registry)

	// Client and modelmanager facades are required for the migration
	// master to function correctly. Missing a model manager causes the
	// status to error out.
	client.Register(registry)
	modelmanager.Register(registry)

	// The following are required to keep the agent alive during
	// migration.
	// This list is extremely conservative, and should be trimmed down
	// once we have a better idea of what is actually required.
	agent.Register(registry)
	caasadmission.Register(registry)
	caasagent.Register(registry)
	caasapplication.Register(registry)
	credentialvalidator.Register(registry)
	deployer.Register(registry)
	diskmanager.Register(registry)
	hostkeyreporter.Register(registry)
	instancemutater.Register(registry)
	keyupdater.Register(registry)
	leadership.Register(registry)
	agentlifeflag.Register(registry)
	loggerapi.Register(registry)
	machine.Register(registry)
	machineactions.Register(registry)
	migrationflag.Register(registry)
	migrationminion.Register(registry)
	payloadshookcontext.Register(registry)
	provisioner.Register(registry)
	proxyupdater.Register(registry)
	reboot.Register(registry)
	resourceshookcontext.Register(registry)
	retrystrategy.Register(registry)
	secretsdrain.Register(registry)
	secretsmanager.Register(registry)
	storageprovisioner.Register(registry)
	unitassigner.Register(registry)
	uniter.Register(registry)
	upgrader.Register(registry)

	registerWatchers(registry)

	list := registry.List()
	versions := make(facades.FacadeVersions, len(list))
	for _, details := range list {
		versions[details.Name] = details.Versions
	}
	return versions
}

// AllFacades returns a registry containing all known API facades.
//
// This will panic if facade registration fails, but there is a unit
// test to guard against that.
func AllFacades() *facade.Registry {
	registry := new(facade.Registry)

	action.Register(registry)
	agent.Register(registry)
	agenttools.Register(registry)
	annotations.Register(registry)
	application.Register(registry)
	applicationoffers.Register(registry)
	backups.Register(registry)
	block.Register(registry)
	bundle.Register(registry)
	charms.Register(registry)
	cleaner.Register(registry)
	client.Register(registry)
	cloud.Register(registry)
	agentlifeflag.Register(registry)

	// CAAS related facades.
	caasadmission.Register(registry)
	caasagent.Register(registry)
	caasapplication.Register(registry)
	caasapplicationprovisioner.Register(registry)
	caasmodeloperator.Register(registry)
	caasmodelconfigmanager.Register(registry)
	caasoperatorupgrader.Register(registry)

	controller.Register(registry)
	crossmodelrelations.Register(registry)
	crossmodelsecrets.Register(registry)
	crosscontroller.Register(registry)
	credentialmanager.Register(registry)
	credentialvalidator.Register(registry)
	externalcontrollerupdater.Register(registry)
	deployer.Register(registry)
	diskmanager.Register(registry)
	firewaller.Register(registry)
	highavailability.Register(registry)
	hostkeyreporter.Register(registry)
	imagemetadata.Register(registry)
	imagemetadatamanager.Register(registry)
	instancemutater.Register(registry)
	keymanager.Register(registry)
	keyupdater.Register(registry)
	leadership.Register(registry)
	loggerapi.Register(registry)
	machineactions.Register(registry)
	machinemanager.Register(registry)
	machineundertaker.Register(registry)
	machine.Register(registry)
	migrationflag.Register(registry)
	migrationmaster.Register(registry)
	migrationminion.Register(registry)
	migrationtarget.Register(requiredMigrationFacadeVersions())(registry)
	modelconfig.Register(registry)
	modelmanager.Register(registry)
	modelupgrader.Register(registry)
	payloadshookcontext.Register(registry)
	pinger.Register(registry)
	provisioner.Register(registry)
	proxyupdater.Register(registry)
	reboot.Register(registry)
	remoterelations.Register(registry)
	resources.Register(registry)
	resourceshookcontext.Register(registry)
	retrystrategy.Register(registry)
	secrets.Register(registry)
	secretbackends.Register(registry)
	secretbackendmanager.Register(registry)
	secretsmanager.Register(registry)
	secretsdrain.Register(registry)
	usersecrets.Register(registry)
	usersecretsdrain.Register(registry)
	sshclient.Register(registry)
	spaces.Register(registry)
	storage.Register(registry)
	storageprovisioner.Register(registry)
	subnets.Register(registry)
	undertaker.Register(registry)
	unitassigner.Register(registry)
	uniter.Register(registry)
	upgrader.Register(registry)
	usermanager.Register(registry)

	registerWatchers(registry)

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

func registerWatchers(registry *facade.Registry) {
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
	registry.MustRegister("SecretsTriggerWatcher", 1, newSecretsTriggerWatcher, reflect.TypeOf((*srvSecretTriggerWatcher)(nil)))
	registry.MustRegister("SecretBackendsRotateWatcher", 1, newSecretBackendsRotateWatcher, reflect.TypeOf((*srvSecretBackendsRotateWatcher)(nil)))
	registry.MustRegister("SecretsRevisionWatcher", 1, newSecretsRevisionWatcher, reflect.TypeOf((*srvSecretsRevisionWatcher)(nil)))
}
