// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see COPYING and COPYING.LESSER file for details.

package goose

import (
	"fmt"
)

type VersionNum struct {
	Major int
	Minor int
	Micro int
}

func (v *VersionNum) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Micro)
}

var VersionNumber = VersionNum{
	Major: 0,
	Minor: 1,
	Micro: 0,
}

var Version = VersionNumber.String()
