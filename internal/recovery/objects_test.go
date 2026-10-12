// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"

	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/recovery"
)

// writeStoreBlob places one blob below dataDir's object store root and
// returns its path.
func writeStoreBlob(c *tc.C, dataDir, namespace, hash, content string) string {
	dir := filepath.Join(dataDir, "objectstore", namespace)
	c.Assert(os.MkdirAll(dir, 0o700), tc.ErrorIsNil)
	blob := filepath.Join(dir, hash)
	c.Assert(os.WriteFile(blob, []byte(content), 0o600), tc.ErrorIsNil)
	return blob
}

func (s *loadSuite) TestCheckObjectStoreComplete(c *tc.C) {
	dataDir := c.MkDir()
	writeStoreBlob(c, dataDir, "model-ns", "somehash", "charm blob content")

	err := recovery.CheckObjectStore(c.Context(), []domainrecovery.ObjectInventory{{
		Namespace: "model-ns",
		Paths: []domainrecovery.ObjectMetadataPath{
			{Path: "charms/foo", SHA384: "somehash"},
		},
	}}, dataDir)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *loadSuite) TestCheckObjectStoreMissingBlobFails(c *tc.C) {
	dataDir := c.MkDir()

	err := recovery.CheckObjectStore(c.Context(), []domainrecovery.ObjectInventory{{
		Namespace: "controller",
		Paths: []domainrecovery.ObjectMetadataPath{
			{Path: "charms/foo", SHA384: "missinghash"},
		},
	}}, dataDir)
	c.Assert(err, tc.ErrorMatches,
		`recovered object metadata references blobs that are not in the archive: controller/charms/foo`)
}

func (s *loadSuite) TestCheckObjectStoreAggregatesMissingAcrossNamespaces(c *tc.C) {
	dataDir := c.MkDir()
	writeStoreBlob(c, dataDir, "ns-a", "presenthash", "content")

	err := recovery.CheckObjectStore(c.Context(), []domainrecovery.ObjectInventory{
		{
			Namespace: "ns-a",
			Paths: []domainrecovery.ObjectMetadataPath{
				{Path: "charms/ok", SHA384: "presenthash"},
				{Path: "charms/gone", SHA384: "absenthash"},
			},
		},
		{
			Namespace: "ns-b",
			Paths: []domainrecovery.ObjectMetadataPath{
				{Path: "agent-binaries/4.0", SHA384: "otherhash"},
			},
		},
	}, dataDir)
	c.Assert(err, tc.ErrorMatches,
		`recovered object metadata references blobs that are not in the archive: ns-a/charms/gone; ns-b/agent-binaries/4.0`)
}

func (s *loadSuite) TestCheckObjectStoreNoInventory(c *tc.C) {
	// A controller whose metadata records no objects — fresh bootstrap
	// archives — passes with an empty store.
	err := recovery.CheckObjectStore(c.Context(), nil, c.MkDir())
	c.Assert(err, tc.ErrorIsNil)
}
