// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type manifestSuite struct {
	testhelpers.CleanupSuite
}

func TestManifestSuite(t *testing.T) {
	tc.Run(t, &manifestSuite{})
}

func (s *manifestSuite) TestReadManifest(c *tc.C) {
	manifest, err := ReadManifest(strings.NewReader(`
bases:
  - name: ubuntu
    channel: "18.04"
    architectures: ["amd64","aarch64","s390x"]
  - name: ubuntu
    channel: "20.04/stable"
`))
	c.Assert(err, tc.IsNil)
	c.Assert(manifest, tc.DeepEquals, &Manifest{Bases: []Base{{
		Name: "ubuntu",
		Channel: Channel{
			Track:  "18.04",
			Risk:   "stable",
			Branch: "",
		},
		Architectures: []string{"amd64", "arm64", "s390x"},
	}, {
		Name: "ubuntu",
		Channel: Channel{
			Track:  "20.04",
			Risk:   "stable",
			Branch: "",
		},
	},
	}})
}

func (s *manifestSuite) TestReadValidateManifest(c *tc.C) {
	_, err := ReadManifest(strings.NewReader(`
bases:
  - name: ""
    channel: "18.04"
`))
	c.Assert(err, tc.ErrorMatches, "manifest: base without name not valid")
}

func (s *manifestSuite) TestValidateManifest(c *tc.C) {
	manifest := &Manifest{
		Bases: []Base{{
			Name: "",
		}},
	}
	c.Assert(manifest.Validate(), tc.ErrorMatches, "validating manifest: base without name not valid")
}
