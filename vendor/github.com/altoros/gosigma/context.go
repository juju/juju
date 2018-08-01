// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"fmt"

	"github.com/altoros/gosigma/data"
)

// A Context interface represents server instance context in CloudSigma account
type Context interface {
	// CloudSigma resource
	Resource

	// CPU frequency in MHz
	CPU() int64

	// Get meta-information value stored in the server instance
	Get(key string) (string, bool)

	// Mem capacity in bytes
	Mem() int64

	// Name of server instance
	Name() string

	// NICs for this context instance
	NICs() []ContextNIC

	// VNCPassword to access the server
	VNCPassword() string
}

// A context implements server instance context in CloudSigma account
type context struct {
	obj *data.Context
}

var _ Context = context{}

// String method is used to print values passed as an operand to any format that
// accepts a string or to an unformatted printer such as Print.
func (c context) String() string {
	return fmt.Sprintf("{Name: %q\nUUID: %q}", c.Name(), c.UUID())
}

// URI of instance
func (c context) URI() string { return fmt.Sprintf("/api/2.0/servers/%s/", c.UUID()) }

// UUID of server instance
func (c context) UUID() string { return c.obj.UUID }

// CPU frequency in MHz
func (c context) CPU() int64 { return c.obj.CPU }

// Get meta-information value stored in the server instance
func (c context) Get(key string) (v string, ok bool) {
	v, ok = c.obj.Meta[key]
	return
}

// Mem capacity in bytes
func (c context) Mem() int64 { return c.obj.Mem }

// Name of server instance
func (c context) Name() string { return c.obj.Name }

// NICs for this context instance
func (c context) NICs() []ContextNIC {
	r := make([]ContextNIC, 0, len(c.obj.NICs))
	for i := range c.obj.NICs {
		nic := contextNIC{&c.obj.NICs[i]}
		r = append(r, nic)
	}
	return r
}

// VNCPassword to access the server
func (c context) VNCPassword() string { return c.obj.VNCPassword }
