// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
)

// Port identifies a network port number for a particular protocol.
//
// NOTE(dimitern): This is deprecated and should be removed, use
// PortRange instead. There are a few places which still use Port,
// especially in apiserver/params, so it can't be removed yet.
type Port struct {
	Protocol string
	Number   int
}

// String implements Stringer.
func (p Port) String() string {
	return fmt.Sprintf("%d/%s", p.Number, p.Protocol)
}

// GoString implements fmt.GoStringer.
func (p Port) GoString() string {
	return p.String()
}
