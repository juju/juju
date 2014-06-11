// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

var (
	ValidateNewFacade       = validateNewFacade
	WrapNewFacade           = wrapNewFacade
	DescriptionFromVersions = descriptionFromVersions
	NilFacadeRecord         = facadeRecord{}
)

type Patcher interface {
	PatchValue(dest, value interface{})
}

func PatchFacades(patcher Patcher) {
	emptyFacades := &FacadeRegistry{}
	patcher.PatchValue(&Facades, emptyFacades)
}
