// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

func NewCharmResource(c *gc.C, name, suffix, comment, fingerprint string) charmresource.Resource {
	var fp charmresource.Fingerprint
	if fingerprint == "" {
		built, err := charmresource.GenerateFingerprint([]byte(name))
		c.Assert(err, jc.ErrorIsNil)
		fp = built
	} else {
		wrapped, err := charmresource.NewFingerprint([]byte(fingerprint))
		c.Assert(err, jc.ErrorIsNil)
		fp = wrapped
	}

	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:    name,
			Type:    charmresource.TypeFile,
			Path:    name + suffix,
			Comment: comment,
		},
		Origin:      charmresource.OriginUpload,
		Revision:    0,
		Fingerprint: fp,
	}
	err := res.Validate()
	c.Assert(err, jc.ErrorIsNil)
	return res
}

func NewCharmResources(c *gc.C, names ...string) []charmresource.Resource {
	var resources []charmresource.Resource
	for _, name := range names {
		var comment string
		parts := strings.SplitN(name, ":", 2)
		if len(parts) == 2 {
			name = parts[0]
			comment = parts[1]
		}

		res := NewCharmResource(c, name, ".tgz", comment, "")
		resources = append(resources, res)
	}
	return resources
}
