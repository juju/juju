// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// SecAssociation is a security association, a relationship between a security
// list and the vcable of an instance. The vcable is an attachment
// point to a specific network interface of an instance.
// The vcable enables dynamic association/disassociation of its instance
// with security lists. You can associate up to eight security lists
// with a given vcable. This limit includes security lists specified in
// the instance configuration (for example, in a launch plan)
// as well as new security associations added after creating instances.
// Note that when a security list is deleted, all the security associations
// involving that security list are also deleted.
type SecAssociation struct {
	// Name is the name of the secure
	// association
	Name string `json:"name"`

	// Seclist is the security list that
	// you want to associate with the instance.
	Seclist string `json:"seclist"`

	// Vcable of the instance that you want to
	// associate with the security list.
	Vcable common.VcableID `json:"vcable"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

// AllSecAssociations is a list of all seurity association
// in the oracle cloud account
type AllSecAssociations struct {
	Result []SecAssociation `json:"result,omitempty"`
}
