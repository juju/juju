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

// SanitizeFacades patches Facades so that for the lifetime of the test we get
// a clean slate to work from, and will not accidentally overrite/mutate the
// real facade registry.
func SanitizeFacades(patcher Patcher) {
	emptyFacades := &FacadeRegistry{}
	patcher.PatchValue(&Facades, emptyFacades)
}
