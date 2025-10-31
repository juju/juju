// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/application/service/storage"
	domainnetwork "github.com/juju/juju/domain/network"
	internalcharm "github.com/juju/juju/internal/charm"
)

// StorageDirectiveOverrides represents override instructions for application
// storage directives to alter the default values a new application will
// receive.
type StorageDirectiveOverrides = storage.StorageDirectiveOverride

type StorageService interface {
	// GetApplicationStorageDirectives returns the storage directives that are
	// set for an application. If the application does not have any storage
	// directives set then an empty result is returned.
	//
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.ApplicationNotFound]
	// when the application no longer exists.
	GetApplicationStorageDirectives(
		context.Context, coreapplication.UUID,
	) ([]application.StorageDirective, error)

	// MakeRegisterExistingCAASUnitStorageArg is responsible for constructing the
	// storage arguments for registering an existing caas unit in the model. This
	// ends up being a set of arguments that are making sure eventual consistency
	// of the unit's storage.MakeRegisterExistingCAASUnitStorageArg
	//
	// The following errors may be expected:
	// - [applicationerrors.UnitNotFound] when the unit no longer exists.
	MakeRegisterExistingCAASUnitStorageArg(
		ctx context.Context,
		unitUUID coreunit.UUID,
		attachmentNetNodeUUID domainnetwork.NetNodeUUID,
		providerFilesystemInfo []caas.FilesystemInfo,
	) (internal.RegisterUnitStorageArg, error)

	// MakeRegisterNewCAASUnitStorageArg is responsible for constructing the storage
	// arguments for registering a new caas unit in the model.
	//
	// The following errors may be expected:
	// - [applicationerrors.ApplicationNotFound] when the application no longer
	// exists.
	MakeRegisterNewCAASUnitStorageArg(
		ctx context.Context,
		appUUID coreapplication.UUID,
		attachmentNetNodeUUID domainnetwork.NetNodeUUID,
		providerFilesystemInfo []caas.FilesystemInfo,
	) (internal.RegisterUnitStorageArg, error)

	// MakeApplicationStorageDirectiveArgs creates a slice of
	// [application.CreateApplicationStorageDirectiveArg] from a set of overrides
	// and the charm storage information. The resultant directives are a merging of
	// all the data sources to form an approximation of what the storage directives
	// for an application should be.
	//
	// The directives SHOULD still be validated.
	MakeApplicationStorageDirectiveArgs(
		ctx context.Context,
		directiveOverrides map[string]storage.StorageDirectiveOverride,
		charmMetaStorage map[string]internalcharm.Storage,
	) ([]internal.CreateApplicationStorageDirectiveArg, error)

	// MakeUnitStorageArgs creates the storage arguments required for a unit in
	// the model. This func looks at the set of directives for the unit and the
	// existing storage available. From this any new instances that need to be
	// created are calculated and all storage attachments are added.
	//
	// The attach netnode uuid argument tell this func what enitities are being
	// attached to in the model.
	//
	// Existing storage supplied to this function will not be included in the
	// storage ownership of the unit. It is expected the unit owns or will own
	// this storage.
	//
	// No guarantee is made that existing storage supplied to this func will be
	// used in its entirety. If a storage directive has less demand then what
	// is supplied it is possible that some existing storage will be unused. It
	// is up to the caller to validate what storage was and wasn't used by
	// looking at the storage attachments.
	MakeUnitStorageArgs(
		ctx context.Context,
		attachNetNodeUUID domainnetwork.NetNodeUUID,
		storageDirectives []application.StorageDirective,
		existingStorage []internal.StorageInstanceComposition,
	) (internal.CreateUnitStorageArg, error)

	// MakeIAASUnitStorageArgs returns [internal.CreateIAASUnitStorageArg] that
	// complement the unit storage arguments provided for IAAS units.
	MakeIAASUnitStorageArgs(
		ctx context.Context,
		unitStorageArg internal.CreateUnitStorageArg,
	) (internal.CreateIAASUnitStorageArg, error)

	// ValidateApplicationStorageDirectiveOverrides checks a set of storage
	// directive overrides to make sure they are valid with respect to the charms
	// storage definitions.
	ValidateApplicationStorageDirectiveOverrides(
		ctx context.Context,
		charmStorageDefs map[string]internalcharm.Storage,
		overrides map[string]storage.StorageDirectiveOverride,
	) error

	// ValidateCharmStorage is responsible for iterating over all of a charms
	// storage requirements and making sure they are valid for deploying as an
	// application.
	//
	// The following errors may be returned:
	// - [domainapplicationerrors].CharmStorageLocationProhibited when one of
	// the charms storage definitions request a location that is prohibited by
	// Juju.
	ValidateCharmStorage(
		ctx context.Context,
		charmStorageDefs map[string]internalcharm.Storage,
	) error
}
