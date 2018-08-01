// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"fmt"

	"github.com/altoros/gosigma/data"
)

// A IPv4 interface represents IPv4 configuration
type IPv4 interface {
	// Convert to string
	fmt.Stringer

	// Configuration type
	Conf() string

	// Resource of IPv4
	Resource() Resource
}

// A ipv4 implements IPv4 configuration
type ipv4 struct {
	client *Client
	obj    *data.IPv4
}

var _ IPv4 = ipv4{}

// String method implements fmt.Stringer interface
func (i ipv4) String() string {
	return fmt.Sprintf("{Conf: %q, %v}", i.Conf(), i.Resource())
}

// Configuration type
func (i ipv4) Conf() string { return i.obj.Conf }

// Resource of IPv4
func (i ipv4) Resource() Resource {
	if i.obj.IP != nil {
		return resource{i.obj.IP}
	}
	return nil
}
