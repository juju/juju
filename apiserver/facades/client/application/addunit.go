// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/core/instance"
	domainapplicationservice "github.com/juju/juju/domain/application/service"
	domainstorage "github.com/juju/juju/domain/storage"
)

// makeIAASAddUnitArgs creates [n] add iaas unit args taking placement and
// existing storage instances to attach to each unit. Both [placement] and
// [storageInstancesToAttach] are position dependent, as in, the zeroth unit to
// be added takes the zeroth placement and storage.
func makeIAASAddUnitArgs(
	n int,
	placement []*instance.Placement,
	storageInstancesToAttach [][]domainstorage.StorageInstanceUUID,
) []domainapplicationservice.AddIAASUnitArg {
	retVal := make([]domainapplicationservice.AddIAASUnitArg, 0, n)
	auas := makeAddUnitArgs(n, placement, storageInstancesToAttach)
	for _, aua := range auas {
		val := domainapplicationservice.AddIAASUnitArg{
			AddUnitArg: aua,
		}
		retVal = append(retVal, val)
	}
	return retVal
}

// makeCAASAddUnitArgs creates [n] add caas unit args taking placement and existing
// storage instances to attach to each unit. Both [placement] and
// [storageInstancesToAttach] are position dependent, as in, the zeroth unit to
// be added takes the zeroth placement and storage.
func makeCAASAddUnitArgs(
	n int,
	placement []*instance.Placement,
	storageInstancesToAttach [][]domainstorage.StorageInstanceUUID,
) []domainapplicationservice.AddUnitArg {
	return makeAddUnitArgs(n, placement, storageInstancesToAttach)
}

// makeAddUnitArgs creates [n] add unit args taking placement and existing
// storage instances to attach to each unit. Both [placement] and
// [storageInstancesToAttach] are position dependent, as in, the zeroth unit to
// be added takes the zeroth placement and storage.
func makeAddUnitArgs(
	n int,
	placement []*instance.Placement,
	storageInstancesToAttach [][]domainstorage.StorageInstanceUUID,
) []domainapplicationservice.AddUnitArg {
	retVal := make([]domainapplicationservice.AddUnitArg, 0, n)
	for i := range n {
		val := domainapplicationservice.AddUnitArg{}
		if i < len(placement) {
			val.Placement = placement[i]
		}
		if i < len(storageInstancesToAttach) {
			val.StorageInstancesToAttach = storageInstancesToAttach[i]
		}
		retVal = append(retVal, val)
	}
	return retVal
}
