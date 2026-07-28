// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/collections/set"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/cloudimagemetadata"
	keymanagerservice "github.com/juju/juju/domain/keymanager/service"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/domain/relation"
)

// ModelService provides access to the model service.
type ModelService interface {
	// GetAllModels returns all models registered in the controller. If no
	// models exist a zero value slice will be returned.
	GetAllModels(context.Context) ([]coremodel.Model, error)
	// Model returns the model associated with the provided uuid.
	Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error)
}

// ModelMigrationService provides access to migration status.
type ModelMigrationService interface {
	// ModelMigrationMode returns the current migration mode for the model.
	ModelMigrationMode(ctx context.Context) (modelmigration.MigrationMode, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

// UpgradeService provides access to upgrade information.
type UpgradeService interface {
	IsUpgrading(context.Context) (bool, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// CheckApplicationsForMigration checks that all applications are ready
	// for migration. All applications and units in the model are alive and no
	// units are in the process of upgrading.
	CheckApplicationsForMigration(ctx context.Context) error

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	GetUnitNamesForApplication(ctx context.Context, appName string) ([]unit.Name, error)
}

// RelationService provides access to the relation service.
type RelationService interface {
	// GetAllRelationDetails return RelationDetailResults for all relations
	// for the current model.
	GetAllRelationDetails(ctx context.Context) ([]relation.RelationDetailsResult, error)

	// RelationUnitInScopeByID returns a boolean to indicate whether the given
	// unit is in scopen of a given relation
	RelationUnitInScopeByID(ctx context.Context, relationID int, unitName unit.Name) (bool,
		error)
}

// StatusService provides access to the statuses service.
type StatusService interface {
	// CheckUnitStatusesReadyForMigration returns true is the statuses of all units
	// in the model indicate they can be migrated.
	CheckUnitStatusesReadyForMigration(context.Context) error

	// CheckMachineStatusesReadyForMigration returns an error if the statuses of any
	// machines in the model indicate they cannot be migrated.
	CheckMachineStatusesReadyForMigration(context.Context) error
}

// ControllerConfigService describes the method needed to get the
// controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// MachineService is used to get the life of all machines before migrating.
type MachineService interface {
	// AllMachineNames returns the names of all machines in the model.
	AllMachineNames(ctx context.Context) ([]machine.Name, error)
	// GetMachineLife returns the GetMachineLife status of the specified machine.
	// It returns a NotFound if the given machine doesn't exist.
	GetMachineLife(ctx context.Context, machineName machine.Name) (life.Value, error)
	// GetMachineBase returns the base for the given machine.
	//
	// The following errors may be returned:
	// - [machineerrors.MachineNotFound] if the machine does not exist.
	GetMachineBase(ctx context.Context, mName machine.Name) (base.Base, error)
}

// CloudService provides access to the cloud service.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
	// ListAll returns all the clouds.
	ListAll(ctx context.Context) ([]cloud.Cloud, error)
}

// The interfaces below are the controller-scoped domain services the v8 import
// driver calls. They are declared here, and constructed above the import
// coordinator rather than inside it, so that every import dependency can be
// substituted in a test. Each one lists only the methods the import actually
// uses.

// ImportClaimService owns the durable import claim: the row that records this
// controller's ownership of a model while it is being imported.
type ImportClaimService interface {
	// BeginImport creates the claim, as the first target-side write of an
	// import, and returns its UUID.
	BeginImport(ctx context.Context, modelUUID coremodel.UUID, sourceMigrationUUID string) (string, error)

	// AssertImporting returns an error unless the claim still exists and is
	// still in the importing phase.
	AssertImporting(ctx context.Context, modelUUID coremodel.UUID) error

	// ImportOfferPermissions records the offers this import granted permissions
	// on, so an abort can find those permission rows again.
	ImportOfferPermissions(ctx context.Context, modelUUID coremodel.UUID, claimUUID string, offerUUIDs []string) error

	// GetImportedOfferUUIDs returns the offers recorded for this import.
	GetImportedOfferUUIDs(ctx context.Context, modelUUID coremodel.UUID) ([]string, error)

	// ImportExternalControllers records the third-party controllers this model
	// consumes offers from.
	ImportExternalControllers(ctx context.Context, modelUUID coremodel.UUID, claimUUID string, refs []coremodelmigration.ExternalController) error
}

// ImportAccessService owns users and permissions on the target controller.
type ImportAccessService interface {
	// ImportModelUsers resolves the model's users against this controller,
	// returning the names that are not usable here.
	ImportModelUsers(ctx context.Context, users []coremodelmigration.ModelUser) (set.Strings, error)

	// ImportModelPermissions applies the model and offer permission grants,
	// returning the offer UUIDs it granted on.
	ImportModelPermissions(ctx context.Context, perms []coremodelmigration.ModelPermission, inactiveUsers set.Strings) ([]string, error)

	// ImportLastModelLogins records when each user last used the model.
	ImportLastModelLogins(ctx context.Context, modelUUID coremodel.UUID, users []coremodelmigration.ModelUser, inactiveUsers set.Strings) error

	// GetUserUUIDByName resolves a username to its UUID on this controller.
	GetUserUUIDByName(ctx context.Context, name user.Name) (user.UUID, error)

	// DeletePermissionsByGrantOnUUID removes every permission granted on the
	// given UUIDs, used to undo an import's permission writes.
	DeletePermissionsByGrantOnUUID(ctx context.Context, grantOnUUIDs []string) error
}

// ImportCredentialService owns the model's cloud credential.
type ImportCredentialService interface {
	// ImportModelCredential resolves the model's credential against this
	// controller, returning the key it resolved to.
	ImportModelCredential(ctx context.Context, ref coremodelmigration.ModelCloudCredential) (credential.Key, error)
}

// ImportKeyManagerService owns the SSH keys authorised for the model.
type ImportKeyManagerService interface {
	// ImportAuthorizedKeys adds the model's authorised keys, resolving each
	// owner through the supplied resolver.
	ImportAuthorizedKeys(ctx context.Context, keys []coremodelmigration.ModelAuthorizedKey, inactiveUsers set.Strings, resolveUser keymanagerservice.UserUUIDResolver) error

	// DeleteKeysForModel removes every authorised key for the model, used to
	// undo an import's key writes.
	DeleteKeysForModel(ctx context.Context) error
}

// ImportSecretBackendService owns the model's references into secret backends.
type ImportSecretBackendService interface {
	// ImportSecretBackendReferences records which backend holds each of the
	// model's secret revisions.
	ImportSecretBackendReferences(ctx context.Context, modelUUID coremodel.UUID, refs []coremodelmigration.SecretBackendReference) error
}

// ImportLeaseService owns application leadership on the target controller.
type ImportLeaseService interface {
	// ImportApplicationLeadership claims fresh leadership leases for the
	// model's applications.
	ImportApplicationLeadership(ctx context.Context, modelUUID coremodel.UUID, leaders []coremodelmigration.ApplicationLeadership) error

	// DeleteLeadershipForModel releases the model's leadership leases, used to
	// undo an import's leadership writes.
	DeleteLeadershipForModel(ctx context.Context, modelUUID coremodel.UUID) error
}

// ImportCloudImageMetadataService owns custom cloud image metadata.
type ImportCloudImageMetadataService interface {
	// ImportCloudImageMetadata recreates the model's custom image metadata,
	// reporting any rows that conflict with metadata already on this
	// controller.
	ImportCloudImageMetadata(ctx context.Context, rows []coremodelmigration.CloudImageMetadata) ([]cloudimagemetadata.MetadataConflict, error)
}
