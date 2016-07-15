// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"fmt"

	"github.com/altoros/gosigma/data"
)

// A RuntimeNIC interface represents runtime information for network interface card
type RuntimeNIC interface {
	// Convert to string
	fmt.Stringer

	// IPv4 configuration
	IPv4() Resource

	// Type of network interface card (public, private, etc)
	Type() string
}

// A runtimeNIC implements runtime information for network interface card
type runtimeNIC struct {
	obj *data.RuntimeNetwork
}

var _ RuntimeNIC = runtimeNIC{}

// String method is used to print values passed as an operand to any format that
// accepts a string or to an unformatted printer such as Print.
func (r runtimeNIC) String() string {
	return fmt.Sprintf(`{Type: %q, IPv4: %v}`, r.Type(), r.IPv4())
}

// IPv4 configuration
func (r runtimeNIC) IPv4() Resource {
	if r.obj.IPv4 != nil {
		return resource{r.obj.IPv4}
	}
	return nil
}

// Type of network interface card (public, private, etc)
func (r runtimeNIC) Type() string { return r.obj.InterfaceType }
