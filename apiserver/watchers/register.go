// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("AllWatcher", 3, NewAllWatcher, reflect.TypeOf((*srvAllWatcher)(nil)))
	// Note: AllModelWatcher uses the same infrastructure as AllWatcher
	// but they are get under separate names as it possible the may
	// diverge in the future (especially in terms of authorisation
	// checks).
	registry.MustRegister("AllModelWatcher", 4, NewAllWatcher, reflect.TypeOf((*srvAllWatcher)(nil)))
	registry.MustRegister("NotifyWatcher", 1, NewNotifyWatcher, reflect.TypeOf((*srvNotifyWatcher)(nil)))
	registry.MustRegister("StringsWatcher", 1, NewStringsWatcher, reflect.TypeOf((*srvStringsWatcher)(nil)))
	registry.MustRegister("OfferStatusWatcher", 1, NewOfferStatusWatcher, reflect.TypeOf((*srvOfferStatusWatcher)(nil)))
	registry.MustRegister("RelationStatusWatcher", 1, NewRelationStatusWatcher, reflect.TypeOf((*srvRelationStatusWatcher)(nil)))
	registry.MustRegister("RelationUnitsWatcher", 1, NewRelationUnitsWatcher, reflect.TypeOf((*srvRelationUnitsWatcher)(nil)))
	registry.MustRegister("RemoteRelationWatcher", 1, NewRemoteRelationWatcher, reflect.TypeOf((*srvRemoteRelationWatcher)(nil)))
	registry.MustRegister("VolumeAttachmentsWatcher", 2, NewVolumeAttachmentsWatcher, reflect.TypeOf((*srvMachineStorageIDsWatcher)(nil)))
	registry.MustRegister("VolumeAttachmentPlansWatcher", 1, NewVolumeAttachmentPlansWatcher, reflect.TypeOf((*srvMachineStorageIDsWatcher)(nil)))
	registry.MustRegister("FilesystemAttachmentsWatcher", 2, NewFilesystemAttachmentsWatcher, reflect.TypeOf((*srvMachineStorageIDsWatcher)(nil)))
	registry.MustRegister("EntityWatcher", 2, NewEntitiesWatcher, reflect.TypeOf((*srvEntitiesWatcher)(nil)))
	registry.MustRegister("MigrationStatusWatcher", 1, NewMigrationStatusWatcher, reflect.TypeOf((*srvMigrationStatusWatcher)(nil)))
	registry.MustRegister("ModelSummaryWatcher", 1, NewModelSummaryWatcher, reflect.TypeOf((*srvModelSummaryWatcher)(nil)))
	registry.MustRegister("SecretsTriggerWatcher", 1, NewSecretsTriggerWatcher, reflect.TypeOf((*srvSecretTriggerWatcher)(nil)))
	registry.MustRegister("SecretBackendsRotateWatcher", 1, NewSecretBackendsRotateWatcher, reflect.TypeOf((*srvSecretBackendsRotateWatcher)(nil)))
	registry.MustRegister("SecretsRevisionWatcher", 1, NewSecretsRevisionWatcher, reflect.TypeOf((*srvSecretsRevisionWatcher)(nil)))
}
