// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credential

import (
	"fmt"
)

// ID represents the id of a cloud credential.
type ID struct {
	// Cloud is the cloud name that the credential applies to.
	Cloud string

	// Owner is the owner of the credential.
	Owner string

	// Name is the name of the credential.
	Name string
}

// String implements the stringer interface.
func (i ID) String() string {
	return fmt.Sprintf("%s/%s/%s", i.Cloud, i.Owner, i.Name)
}
