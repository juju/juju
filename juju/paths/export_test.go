// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package paths

func NewMongoTest(binDir string) Mongo {
	return Mongo{binDir}
}
