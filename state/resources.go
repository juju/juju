// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/charmresources"
)

// ResourceManager returns a new charmresources.ResourceManager
// that stores charm resources metadata.
func (st *State) ResourceManager() charmresources.ResourceManager {
	// TODO - wallyworld
	panic("not implemented")
}
