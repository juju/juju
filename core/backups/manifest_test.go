// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
	coreerrors "github.com/juju/juju/core/errors"
)

type manifestSuite struct{}

func TestManifestSuite(t *stdtesting.T) {
	tc.Run(t, &manifestSuite{})
}

func (s *manifestSuite) TestManifestRoundTrip(c *tc.C) {
	entries := []backups.ManifestEntry{{
		Path:   "juju-backup/metadata.json",
		Kind:   backups.ManifestKindMetadata,
		Size:   42,
		SHA256: "abc123",
	}, {
		Path:      "juju-backup/dump/models/deadbeef-0bad-400d-8000-4b1d0d06f00d.yaml",
		Kind:      backups.ManifestKindModelDump,
		Size:      7,
		SHA256:    "def456",
		ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
	}}

	reader, err := backups.NewManifest(entries).AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)
	back, err := backups.NewManifestJSONReader(reader)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(back.FormatVersion, tc.Equals, int64(1))
	c.Check(back.Files, tc.DeepEquals, entries)
}

func (s *manifestSuite) TestManifestJSONReaderInvalid(c *tc.C) {
	_, err := backups.NewManifestJSONReader(strings.NewReader("{"))
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *manifestSuite) TestClassifyManifestPath(c *tc.C) {
	for _, test := range []struct {
		path      string
		kind      backups.ManifestEntryKind
		modelUUID string
	}{
		{"juju-backup/metadata.json", backups.ManifestKindMetadata, ""},
		{"juju-backup/root.tar", backups.ManifestKindFilesBundle, ""},
		{"juju-backup/dump/controller.yaml", backups.ManifestKindControllerDump, ""},
		{
			"juju-backup/dump/models/deadbeef-0bad-400d-8000-4b1d0d06f00d.yaml",
			backups.ManifestKindModelDump,
			"deadbeef-0bad-400d-8000-4b1d0d06f00d",
		},
	} {
		kind, modelUUID, err := backups.ClassifyManifestPath(test.path)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(kind, tc.Equals, test.kind)
		c.Check(modelUUID, tc.Equals, test.modelUUID)
	}
}

// TestClassifyManifestPathUnknown pins the failure mode for paths
// outside the known archive components: an error, never an "unknown"
// bucket a reader would have to handle later.
func (s *manifestSuite) TestClassifyManifestPathUnknown(c *tc.C) {
	for _, path := range []string{
		"juju-backup/dump/other.yaml",
		"juju-backup/dump/models/.yaml",
		"juju-backup/dump/models/sub/x.yaml",
		"juju-backup/manifest.json",
		"juju-backup/extra.bin",
	} {
		_, _, err := backups.ClassifyManifestPath(path)
		c.Check(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("path %q", path))
	}
}
