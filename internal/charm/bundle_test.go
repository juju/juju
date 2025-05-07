// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/charm"
)

func checkWordpressBundle(c *tc.C, b *charm.BundleArchive, path string, bundleName string) {
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
	c.Assert(err, tc.IsNil)

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
	c.Assert(b.ReadMe(), tc.Equals, "A dummy bundle\n")
	c.Assert(b.Path, tc.Equals, path)

	bundlePath := filepath.Join("internal/test-charm-repo/bundle", bundleName, "bundle.yaml")
	raw, err := os.ReadFile(bundlePath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b.BundleBytes()), tc.Equals, string(raw))
}

func verifyOk(string) error {
	return nil
}
