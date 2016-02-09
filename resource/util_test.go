// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
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

func newFullResource(c *gc.C, name string) resource.Resource {
	return resource.Resource{
		Resource:  newFullCharmResource(c, name),
		ID:        "a-service/" + name,
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: time.Now(),
	}
}
