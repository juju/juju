// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
)

// MongoStoragePath returns the path that is used to
// retrieve the given version of mongodb in a Storage.
func MongoStoragePath(series, architecture string) string {
	return fmt.Sprintf("tools/mongo-2.2.0-%s-%s.tgz", series, architecture)
}
