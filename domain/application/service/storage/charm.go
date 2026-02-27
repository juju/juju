// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"slices"
	"strings"

	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
)

// ValidateCharmStorage is responsible for iterating over all of a charms
// storage requirements and making sure they are valid for deploying as an
// application.
//
// The following errors may be returned:
// - [domainapplicationerrors].CharmStorageLocationProhibited when one of the
// charms storage definitions request a location that is prohibited by Juju.
func (s *Service) ValidateCharmStorage(
	ctx context.Context,
	charmStorageDefs map[string]internalcharm.Storage,
) error {
	badLocations := domainstorageprovisioning.ProhibitedFilesystemAttachmentLocations()
	for name, storageDef := range charmStorageDefs {
		location := storageDef.Location
		if location == "" || storageDef.Type == internalcharm.StorageBlock {
			continue
		}

		index := slices.IndexFunc(badLocations, func(b string) bool {
			return strings.HasPrefix(location, b)
		})

		if index != -1 {
			return domainapplicationerrors.CharmStorageLocationProhibited{
				CharmStorageLocation: location,
				CharmStorageName:     name,
				ProhibitedLocation:   badLocations[index],
			}
		}
	}

	return nil
}

// ValidateNewCharmStorageAgainstExistingCharmStorage ensures that the new charm's
// storage requirements are compatible with the existing charm's storage requirements.
// It validates the following rules from the existing charm to the new charm:
//   - No storage has been removed.
//   - Storage type must not change.
//   - Minimum storage size must not increase.
//   - Minimum storage count must not increase.
//   - Maximum storage count must not decrease (unbounded cannot become bounded).
//
// The following errors can be expected:
//   - [domainapplicationerrors.CharmStorageDefinitionRemoved] when storage is removed in new charm.
//   - [domainapplicationerrors.CharmStorageTypeChanged] when type of a storage in new charm changes.
//   - [domainapplicationerrors.CharmStorageDefinitionMinSizeViolation] when minimum size increased in new charm.
//   - [domainapplicationerrors.CharmStorageDefinitionMinCountViolation] when minimum count increased in new charm.
//   - [domainapplicationerrors.CharmStorageDefinitionMaxCountViolation] when maximum count decreased or became bounded from unbounded in new charm.
//   - [domainapplicationerrors.CharmStorageDefinitionSingleToMultipleViolation] when a singleton storage with a fixed location changes to multiple.
//   - [domainapplicationerrors.CharmStorageDefinitionSharedChanged] when shared changes.
//   - [domainapplicationerrors.CharmStorageDefinitionReadOnlyChanged] when read-only changes.
//   - [domainapplicationerrors.CharmStorageDefinitionLocationChanged] when location changes.
func ValidateNewCharmStorageAgainstExistingCharmStorage(
	newCharmStorageReqs map[string]internalcharm.Storage,
	existingCharmStorageReqs map[string]internalcharm.Storage,
) error {
	for storageName, existingStorageReq := range existingCharmStorageReqs {
		newCharmStorageReq, existsInNewCharm := newCharmStorageReqs[storageName]
		if !existsInNewCharm {
			// Storage no longer in new charm, return an error.
			return domainapplicationerrors.CharmStorageDefinitionRemoved{
				StorageName: storageName,
			}
		}
		// Condition 1: Minimum storage size of new charm must not be more than existing charm minimum storage size.
		if newCharmStorageReq.MinimumSize > existingStorageReq.MinimumSize {
			return domainapplicationerrors.CharmStorageDefinitionMinSizeViolation{
				StorageName: storageName,
				ExistingMin: existingStorageReq.MinimumSize,
				NewMin:      newCharmStorageReq.MinimumSize,
			}
		}

		// Condition 2: Storage type of new charm must match existing charm storage type.
		if newCharmStorageReq.Type != existingStorageReq.Type {
			return domainapplicationerrors.CharmStorageTypeChanged{
				StorageName: storageName,
				OldType:     existingStorageReq.Type.String(),
				NewType:     newCharmStorageReq.Type.String(),
			}
		}

		// Condition 3a: Count min of new charm must not be more than existing charm minimum count.
		if newCharmStorageReq.CountMin > existingStorageReq.CountMin {
			return domainapplicationerrors.CharmStorageDefinitionMinCountViolation{
				StorageName: storageName,
				ExistingMin: existingStorageReq.CountMin,
				NewMin:      newCharmStorageReq.CountMin,
			}
		}

		// Condition 3b: Count max of new charm must not tighten count max.
		oldMax := existingStorageReq.CountMax
		newMax := newCharmStorageReq.CountMax

		// Unbounded to bounded is a violation.
		if oldMax < 0 && newMax >= 0 {
			return domainapplicationerrors.CharmStorageDefinitionMaxCountViolation{
				StorageName: storageName,
				ExistingMax: oldMax,
				NewMax:      newMax,
			}
		}

		// Bounded to bounded, ensure new max is not less than old max.
		if newMax >= 0 && oldMax >= 0 && newMax < oldMax {
			return domainapplicationerrors.CharmStorageDefinitionMaxCountViolation{
				StorageName: storageName,
				ExistingMax: oldMax,
				NewMax:      newMax,
			}
		}

		// Condition 4: A singleton storage with a fixed location cannot change to multiple,
		// as the location semantics become incompatible.
		if existingStorageReq.Location != "" && oldMax == 1 && newMax != 1 {
			return domainapplicationerrors.CharmStorageDefinitionSingleToMultipleViolation{
				StorageName: storageName,
				ExistingMax: oldMax,
				NewMax:      newMax,
			}
		}

		// Condition 5: Shared property must not change.
		if newCharmStorageReq.Shared != existingStorageReq.Shared {
			return domainapplicationerrors.CharmStorageDefinitionSharedChanged{
				StorageName:   storageName,
				ExistingValue: existingStorageReq.Shared,
				NewValue:      newCharmStorageReq.Shared,
			}
		}

		// Condition 6: Read-only property must not change.
		if newCharmStorageReq.ReadOnly != existingStorageReq.ReadOnly {
			return domainapplicationerrors.CharmStorageDefinitionReadOnlyChanged{
				StorageName:   storageName,
				ExistingValue: existingStorageReq.ReadOnly,
				NewValue:      newCharmStorageReq.ReadOnly,
			}
		}

		// Condition 7: Location must not change.
		if newCharmStorageReq.Location != existingStorageReq.Location {
			return domainapplicationerrors.CharmStorageDefinitionLocationChanged{
				StorageName:      storageName,
				ExistingLocation: existingStorageReq.Location,
				NewLocation:      newCharmStorageReq.Location,
			}
		}

	}
	return nil
}
