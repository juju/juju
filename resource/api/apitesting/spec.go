// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apitesting

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

func NewSpec(c *gc.C, name string) (resource.Spec, api.ResourceSpec) {
	info := charm.ResourceInfo{
		Name: name,
		Type: charm.ResourceTypeFile,
		Path: name + ".tgz",
	}
	spec, err := resource.NewSpec(info, resource.OriginUpload, "")
	c.Assert(err, jc.ErrorIsNil)

	apiSpec := api.ResourceSpec{
		Name:   name,
		Type:   "file",
		Path:   name + ".tgz",
		Origin: "upload",
	}

	return spec, apiSpec
}
