// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// CreateLXDProfileDetails holds the data need to write an lxd profile
// to a machine via the lxd server API.
type CreateLXDProfileDetails struct {
	ApplicationName string
	CharmRevision   int
	LXDProfile      []byte
}
