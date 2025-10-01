// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

// MakeStorageDirectiveFromApplicationArg is responsible for takeing the storage
// directive create params for an application and converting them into
// [application.StorageDirective] types.
func MakeStorageDirectiveFromApplicationArg(
	charmMetadataName string,
	charmStorage map[string]internalcharm.Storage,
	applicationArgs []application.CreateApplicationStorageDirectiveArg,
) []application.StorageDirective {
	rval := make([]application.StorageDirective, 0, len(applicationArgs))
	for _, arg := range applicationArgs {
		rval = append(rval, application.StorageDirective{
			CharmMetadataName: charmMetadataName,
			Name:              arg.Name,
			Count:             arg.Count,
			CharmStorageType:  charm.StorageType(charmStorage[arg.Name.String()].Type),
			PoolUUID:          arg.PoolUUID,
			Size:              arg.Size,
		})
	}

	return rval
}
