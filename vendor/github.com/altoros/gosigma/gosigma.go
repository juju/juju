// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"errors"
	"fmt"
)

// VersionNum defines library version
type VersionNum struct {
	Major int
	Minor int
	Micro int
}

// String representation of VersionNum object
func (v VersionNum) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Micro)
}

// VersionNumber returns current library version
func VersionNumber() VersionNum {
	return VersionNum{Major: 0, Minor: 1, Micro: 0}
}

// Version returns string with current library version
func Version() string {
	return VersionNumber().String()
}

// ErrOperationTimeout defines error for operation timeout
var ErrOperationTimeout = errors.New("operation timeout")
