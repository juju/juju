// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"crypto/sha512"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

func NewInfo(c *gc.C, name, suffix, comment, fingerprint string) resource.Info {
	if fingerprint == "" {
		hash := sha512.New384()
		_, err := hash.Write([]byte(name))
		c.Assert(err, jc.ErrorIsNil)
		fingerprint = string(hash.Sum(nil))
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
			Fingerprint: fingerprint,
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
