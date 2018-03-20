// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"time"
)

// ExpireAfter returns the expire after target for an ExpirableStorage
func ExpireAfter(s ExpirableStorage) time.Duration {
	return s.(*storage).expireAfter
}
