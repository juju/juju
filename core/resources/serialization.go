// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/charm/resource"
)

// DeserializeFingerprint converts the serialized fingerprint back into
// a Fingerprint. "zero" values are treated appropriately.
func DeserializeFingerprint(fpSum []byte) (resource.Fingerprint, error) {
	var fp resource.Fingerprint
	if len(fpSum) != 0 {
		var err error
		fp, err = resource.NewFingerprint(fpSum)
		if err != nil {
			return fp, errors.Trace(err)
		}
	}
	return fp, nil
}
