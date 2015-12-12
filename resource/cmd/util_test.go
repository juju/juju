// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

func NewInfo(c *gc.C, name, suffix, comment, fingerprint string) resource.Info {
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

	info := resource.Info{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    name,
				Type:    charmresource.TypeFile,
				Path:    name + suffix,
				Comment: comment,
			},
			Revision:    0,
			Fingerprint: fp,
		},
		Origin: resource.OriginKindUpload,
	}
	err := info.Validate()
	c.Assert(err, jc.ErrorIsNil)
	return info
}

func NewInfos(c *gc.C, names ...string) []resource.Info {
	var infos []resource.Info
	for _, name := range names {
		var comment string
		parts := strings.SplitN(name, ":", 2)
		if len(parts) == 2 {
			name = parts[0]
			comment = parts[1]
		}

		info := NewInfo(c, name, ".tgz", comment, "")
		infos = append(infos, info)
	}
	return infos
}
