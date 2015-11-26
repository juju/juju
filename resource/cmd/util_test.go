// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/resource"
)

func NewSpec(c *gc.C, name, suffix, comment string) resource.Spec {
	info := charm.ResourceInfo{
		Name:    name,
		Type:    charm.ResourceTypeFile,
		Path:    name + suffix,
		Comment: comment,
	}
	spec, err := resource.NewSpec(info, resource.OriginUpload, "")
	c.Assert(err, jc.ErrorIsNil)
	return spec
}

func NewSpecs(c *gc.C, names ...string) []resource.Spec {
	var specs []resource.Spec
	for _, name := range names {
		var comment string
		parts := strings.SplitN(name, ":", 2)
		if len(parts) == 2 {
			name = parts[0]
			comment = parts[1]
		}

		spec := NewSpec(c, name, ".tgz", comment)
		specs = append(specs, spec)
	}
	return specs
}
