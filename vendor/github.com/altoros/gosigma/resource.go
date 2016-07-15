// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"fmt"

	"github.com/altoros/gosigma/data"
)

// A Resource interface represents abstract resource in CloudSigma account
type Resource interface {
	// Convert to string
	fmt.Stringer

	// URI of server instance
	URI() string

	// UUID of server instance
	UUID() string
}

// A resource implements abstract CloudSigma resource
type resource struct {
	obj *data.Resource
}

var _ Resource = resource{}

// String method implements fmt.Stringer interface
func (r resource) String() string {
	return fmt.Sprintf("{URI: %q, UUID: %q}", r.URI(), r.UUID())
}

// URI of instance
func (r resource) URI() string { return r.obj.URI }

// UUID of instance
func (r resource) UUID() string { return r.obj.UUID }
