// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"strings"

	charmresource "github.com/juju/charm/v7/resource"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func newFingerprint(c *gc.C, data string) charmresource.Fingerprint {
	reader := strings.NewReader(data)
	fp, err := charmresource.GenerateFingerprint(reader)
	c.Assert(err, jc.ErrorIsNil)
	return fp
}

func newFullCharmResource(c *gc.C, name string) charmresource.Resource {
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
