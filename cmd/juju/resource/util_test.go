// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"bytes"
	"strings"

	charmresource "github.com/juju/charm/v7/resource"
	jujucmd "github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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
	ctx := cmdtesting.Context(c)
	code = jujucmd.Main(command, ctx, args)
	stdout = string(ctx.Stdout.(*bytes.Buffer).Bytes())
	stderr = string(ctx.Stderr.(*bytes.Buffer).Bytes())
	return code, stdout, stderr
}
