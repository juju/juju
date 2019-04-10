// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

var (
	// JujudStartUpSh is the exec script for CAAS controller.
	JujudStartUpSh = `
cp /opt/jujud $(pwd)/jujud
%s
`[1:]
)
