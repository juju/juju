// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/testhelpers"
)

// secretRewriteSuite exercises [migration.RewriteSecretBackendUUIDs].
type secretRewriteSuite struct {
	testhelpers.IsolationSuite
}

func TestSecretRewriteSuite(t *testing.T) {
	tc.Run(t, &secretRewriteSuite{})
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsNilPayload(c *tc.C) {
	err := migration.RewriteSecretBackendUUIDs(nil, map[string]string{"rev-1": "target-id"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsEmptyPayload(c *tc.C) {
	payload := &latest.ModelExport{}
	err := migration.RewriteSecretBackendUUIDs(payload, map[string]string{"rev-1": "target-id"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsEmptyMap(c *tc.C) {
	payload := &latest.ModelExport{
		SecretValueRef: []v4_1_0.SecretValueRef{
			{RevisionUUID: "rev-1", BackendUUID: "source-id"},
		},
	}
	err := migration.RewriteSecretBackendUUIDs(payload, nil)
	c.Assert(err, tc.ErrorMatches, `no target secret backend for secret revision "rev-1"`)
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsRewriteBoth(c *tc.C) {
	payload := &latest.ModelExport{
		SecretValueRef: []v4_1_0.SecretValueRef{
			{RevisionUUID: "rev-1", BackendUUID: "source-1", RevisionID: "rid-1"},
			{RevisionUUID: "rev-2", BackendUUID: "source-2", RevisionID: "rid-2"},
		},
		SecretDeletedValueRef: []v4_1_0.SecretDeletedValueRef{
			{RevisionUUID: "rev-1", BackendUUID: "source-1", RevisionID: "rid-1"},
			{RevisionUUID: "rev-3", BackendUUID: "source-3", RevisionID: "rid-3"},
		},
	}

	revisionMap := map[string]string{
		"rev-1": "target-1",
		"rev-2": "target-2",
		"rev-3": "target-3",
	}

	err := migration.RewriteSecretBackendUUIDs(payload, revisionMap)
	c.Assert(err, tc.ErrorIsNil)

	// Value refs rewritten.
	c.Check(payload.SecretValueRef[0].BackendUUID, tc.Equals, "target-1")
	c.Check(payload.SecretValueRef[0].RevisionID, tc.Equals, "rid-1")
	c.Check(payload.SecretValueRef[1].BackendUUID, tc.Equals, "target-2")
	c.Check(payload.SecretValueRef[1].RevisionID, tc.Equals, "rid-2")

	// Deleted value refs rewritten.
	c.Check(payload.SecretDeletedValueRef[0].BackendUUID, tc.Equals, "target-1")
	c.Check(payload.SecretDeletedValueRef[0].RevisionID, tc.Equals, "rid-1")
	c.Check(payload.SecretDeletedValueRef[1].BackendUUID, tc.Equals, "target-3")
	c.Check(payload.SecretDeletedValueRef[1].RevisionID, tc.Equals, "rid-3")
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsMissingRevision(c *tc.C) {
	payload := &latest.ModelExport{
		SecretValueRef: []v4_1_0.SecretValueRef{
			{RevisionUUID: "rev-1", BackendUUID: "source-1"},
		},
	}

	revisionMap := map[string]string{
		"rev-other": "target-other",
	}

	err := migration.RewriteSecretBackendUUIDs(payload, revisionMap)
	c.Assert(err, tc.ErrorMatches, `no target secret backend for secret revision "rev-1"`)
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsMissingDeletedRevision(c *tc.C) {
	payload := &latest.ModelExport{
		SecretDeletedValueRef: []v4_1_0.SecretDeletedValueRef{
			{RevisionUUID: "rev-1", BackendUUID: "source-1"},
		},
	}

	revisionMap := map[string]string{
		"rev-other": "target-other",
	}

	err := migration.RewriteSecretBackendUUIDs(payload, revisionMap)
	c.Assert(err, tc.ErrorMatches, `no target secret backend for secret revision "rev-1" \(deleted value ref\)`)
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsDistinctBackends(c *tc.C) {
	payload := &latest.ModelExport{
		SecretValueRef: []v4_1_0.SecretValueRef{
			{RevisionUUID: "rev-1", BackendUUID: "src-a"},
			{RevisionUUID: "rev-2", BackendUUID: "src-b"},
		},
	}

	revisionMap := map[string]string{
		"rev-1": "tgt-a",
		"rev-2": "tgt-b",
	}

	err := migration.RewriteSecretBackendUUIDs(payload, revisionMap)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(payload.SecretValueRef[0].BackendUUID, tc.Equals, "tgt-a")
	c.Check(payload.SecretValueRef[1].BackendUUID, tc.Equals, "tgt-b")
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsOnlyValueRefs(c *tc.C) {
	payload := &latest.ModelExport{
		SecretValueRef: []v4_1_0.SecretValueRef{
			{RevisionUUID: "rev-1", BackendUUID: "source-1"},
		},
	}

	revisionMap := map[string]string{"rev-1": "target-1"}

	err := migration.RewriteSecretBackendUUIDs(payload, revisionMap)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(payload.SecretValueRef[0].BackendUUID, tc.Equals, "target-1")
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsOnlyDeletedRefs(c *tc.C) {
	payload := &latest.ModelExport{
		SecretDeletedValueRef: []v4_1_0.SecretDeletedValueRef{
			{RevisionUUID: "rev-1", BackendUUID: "source-1"},
		},
	}

	revisionMap := map[string]string{"rev-1": "target-1"}

	err := migration.RewriteSecretBackendUUIDs(payload, revisionMap)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(payload.SecretDeletedValueRef[0].BackendUUID, tc.Equals, "target-1")
}

func (s *secretRewriteSuite) TestRewriteSecretBackendUUIDsNoRows(c *tc.C) {
	payload := &latest.ModelExport{
		SecretValueRef:        nil,
		SecretDeletedValueRef: nil,
		// Some other payload fields to show it's a real payload.
		Sequence: []v4_1_0.Sequence{{Namespace: "test", Value: 42}},
	}

	revisionMap := map[string]string{"rev-1": "target-1"}

	err := migration.RewriteSecretBackendUUIDs(payload, revisionMap)
	c.Assert(err, tc.ErrorIsNil)
}
