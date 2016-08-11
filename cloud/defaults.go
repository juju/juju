// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

// Defaults describes a set of defaults for cloud, region,
// and credential to use.
type Defaults struct {
	// Cloud is the name of the cloud to use by default.
	Cloud string

	// Region is the name of the cloud region to use by default,
	// if the cloud supports regions.
	Region string

	// Credential is the name of the cloud credential to use
	// by default, if the cloud requires credentials.
	Credential string
}
