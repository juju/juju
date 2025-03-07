// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

func Storage(st *State) map[string]bool {
	return st.storage
}
