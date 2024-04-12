// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
)

// cloudGlobalKey will return the key for a given cloud.
func cloudGlobalKey(cloudName string) string {
	return fmt.Sprintf("cloud#%s", cloudName)
}
