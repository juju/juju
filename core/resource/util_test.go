// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	charmresource "github.com/juju/juju/internal/charm/resource"
)

func newFingerprint(c *tc.C, data string) charmresource.Fingerprint {
	reader := strings.NewReader(data)
	fp, err := charmresource.GenerateFingerprint(reader)
	c.Assert(err, jc.ErrorIsNil)
	return fp
}

func newFullCharmResource(c *tc.C, name string) charmresource.Resource {
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        name,
			Type:        charmresource.TypeFile,
			Path:        name + ".tgz",
			Description: "you need it",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: newFingerprint(c, name),
	}
}
