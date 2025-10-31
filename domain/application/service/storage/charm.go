// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"slices"
	"strings"

	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	internalcharm "github.com/juju/juju/internal/charm"
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
