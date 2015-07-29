// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/charmresources"
	"github.com/juju/juju/state/resourcestorage"
)

// ResourceManager returns a new charmresources.ResourceManager
// that stores charm resources metadata.
func (st *State) ResourceManager() charmresources.ResourceManager {
	return resourcestorage.NewResourceManager(st.session, st.EnvironUUID())
}
