// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"strings"

	jujucmd "github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	coretesting "github.com/juju/juju/testing"
)

func charmRes(c *gc.C, name, suffix, description, content string) charmresource.Resource {
	if content == "" {
		content = name
	}

	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)

	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        name,
			Type:        charmresource.TypeFile,
			Path:        name + suffix,
			Description: description,
		},
		Origin:      charmresource.OriginStore,
		Revision:    1,
		Fingerprint: fp,
		Size:        int64(len(content)),
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)
	return res
}

func newCharmResources(c *gc.C, names ...string) []charmresource.Resource {
	var resources []charmresource.Resource
	for _, name := range names {
		var description string
		parts := strings.SplitN(name, ":", 2)
		if len(parts) == 2 {
			name = parts[0]
			description = parts[1]
		}

		res := charmRes(c, name, ".tgz", description, "")
		resources = append(resources, res)
	}
	return resources
}

func runCmd(c *gc.C, command jujucmd.Command, args ...string) (code int, stdout string, stderr string) {
	ctx := coretesting.Context(c)
	code = jujucmd.Main(command, ctx, args)
	stdout = string(ctx.Stdout.(*bytes.Buffer).Bytes())
	stderr = string(ctx.Stderr.(*bytes.Buffer).Bytes())
	return code, stdout, stderr
}
