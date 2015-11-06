// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package instance

const (
	LXD = ContainerType("lxd")
)

func init() {
	ContainerTypes = append(ContainerTypes, LXD)
}
