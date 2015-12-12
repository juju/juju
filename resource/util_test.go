// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

func newFingerprint(c *gc.C, data string) charmresource.Fingerprint {
	fp, err := charmresource.GenerateFingerprint([]byte(data))
	c.Assert(err, jc.ErrorIsNil)
	return fp
}

func newFullCharmResource(c *gc.C, name string) charmresource.Resource {
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:    name,
			Type:    charmresource.TypeFile,
			Path:    name + ".tgz",
			Comment: "you need it",
		},
		Revision:    1,
		Fingerprint: newFingerprint(c, name),
	}
}

func newFullInfo(c *gc.C, name string) resource.Info {
	return resource.Info{
		Resource: newFullCharmResource(c, name),
		Origin:   resource.OriginKindUpload,
	}
}

func newFullResource(c *gc.C, name string) resource.Resource {
	return resource.Resource{
		Info:      newFullInfo(c, name),
		Username:  "a-user",
		Timestamp: time.Now(),
	}
}
