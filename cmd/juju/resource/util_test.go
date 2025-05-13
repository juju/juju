// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"bytes"
	"strings"

	"github.com/juju/tc"

	charmresource "github.com/juju/juju/internal/charm/resource"
	jujucmd "github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

func charmRes(c *tc.C, name, suffix, description, content string) charmresource.Resource {
	if content == "" {
		content = name
	}

	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)
	return res
}

func newCharmResources(c *tc.C, names ...string) []charmresource.Resource {
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

func runCmd(c *tc.C, command jujucmd.Command, args ...string) (code int, stdout string, stderr string) {
	ctx := cmdtesting.Context(c)
	code = jujucmd.Main(command, ctx, args)
	stdout = string(ctx.Stdout.(*bytes.Buffer).Bytes())
	stderr = string(ctx.Stderr.(*bytes.Buffer).Bytes())
	return code, stdout, stderr
}
