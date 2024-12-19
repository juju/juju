// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charm"
)

func checkWordpressBundle(c *gc.C, b *charm.BundleArchive, path string, bundleName string) {
	// Load the charms required by the bundle.
	wordpressCharm := readCharmDir(c, "wordpress")
	mysqlCharm := readCharmDir(c, "mysql")

	bd := b.Data()
	c.Assert(bd.RequiredCharms(), jc.DeepEquals, []string{"ch:mysql", "ch:wordpress"})

	charms := map[string]charm.Charm{
		"ch:wordpress": wordpressCharm,
		"ch:mysql":     mysqlCharm,
	}
	err := bd.VerifyWithCharms(verifyOk, nil, nil, charms)
	c.Assert(err, gc.IsNil)

	c.Assert(bd.Applications, jc.DeepEquals, map[string]*charm.ApplicationSpec{
		"wordpress": {
			Charm: "ch:wordpress",
		},
		"mysql": {
			Charm:    "ch:mysql",
			NumUnits: 1,
		},
	})
	c.Assert(bd.Relations, jc.DeepEquals, [][]string{
		{"wordpress:db", "mysql:server"},
	})
	c.Assert(b.ReadMe(), gc.Equals, "A dummy bundle\n")
	c.Assert(b.Path, gc.Equals, path)

	bundlePath := filepath.Join("internal/test-charm-repo/bundle", bundleName, "bundle.yaml")
	raw, err := os.ReadFile(bundlePath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b.BundleBytes()), gc.Equals, string(raw))
}

func verifyOk(string) error {
	return nil
}
