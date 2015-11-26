// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"strings"

	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/resource"
)

func NewSpecs(c *gc.C, names ...string) []resource.Spec {
	var specs []resource.Spec
	for _, name := range names {
		var comment string
		parts := strings.SplitN(name, ":", 2)
		if len(parts) == 2 {
			name = parts[0]
			comment = parts[1]
		}

		info := charm.ResourceInfo{
			Name:    name,
			Type:    charm.ResourceTypeFile,
			Path:    name + ".tgz",
			Comment: comment,
		}
		spec, err := resource.NewSpec(info, resource.OriginUpload, "")
		c.Assert(err, jc.ErrorIsNil)
		specs = append(specs, spec)
	}
	return specs
}
