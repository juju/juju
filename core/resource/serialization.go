// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// DeserializeFingerprint converts the serialized fingerprint back into
// a Fingerprint. "zero" values are treated appropriately.
func DeserializeFingerprint(fpSum []byte) (resource.Fingerprint, error) {
	var fp resource.Fingerprint
	if len(fpSum) != 0 {
		var err error
		fp, err = resource.NewFingerprint(fpSum)
		if err != nil {
			return fp, errors.Capture(err)
		}
	}
	return fp, nil
}
