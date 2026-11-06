// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport_test

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/export"
	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/domain/modelimport"
)

func (s *importSuite) TestLegacyLookupPersistenceRoundTrip(c *tc.C) {
	s.bootstrapModel(c)
	exporter := exportstate.NewState(s.TxnRunnerFactory())
	seeded, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	legacy := v4_0_12.ModelExport{
		ModelAgent:         []v4_0_12.ModelAgent{{ModelUUID: s.ModelUUID()}},
		ChangeLogEditType:  []v4_0_12.ChangeLogEditType{{EditType: new("source-only")}},
		ChangeLogNamespace: []v4_0_12.ChangeLogNamespace{{Namespace: new("source-only"), Description: new("source-only")}},
		AgentStream: []v4_0_12.AgentStream{
			{ID: new(int64(0)), Name: "must-not-replace-released"},
			{ID: new(int64(1)), Name: "must-not-replace-proposed"},
		},
		StorageVolumeDeviceType: []v4_0_12.StorageVolumeDeviceType{
			{ID: new(int64(0)), Name: "must-not-replace-local", Description: new("must-not-replace-description")},
			{ID: new(int64(17)), Name: "custom-null"},
			{ID: new(int64(18)), Name: "custom-described", Description: new("custom device description")},
		},
		SecretRole: []v4_0_12.SecretRole{
			{ID: new(int64(0)), Role: new("must-not-replace-none")},
			{ID: new(int64(17))},
			{ID: new(int64(18)), Role: new("custom-role")},
		},
	}
	data, err := yaml.Marshal(legacy)
	c.Assert(err, tc.ErrorIsNil)
	decoded, err := export.DecodePayload(semversion.MustParse("4.0.12"), data)
	c.Assert(err, tc.ErrorIsNil)
	transform, err := modelimport.NewTransformer()
	c.Assert(err, tc.ErrorIsNil)
	transformed, err := transform.Transform(c.Context(), semversion.MustParse("4.0.12"), decoded)
	c.Assert(err, tc.ErrorIsNil)
	payload, ok := transformed.(latest.ModelExport)
	c.Assert(ok, tc.IsTrue)

	for range 2 {
		// Repeated imports must preserve seed rows and avoid duplicates.
		err = modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), &payload)
		c.Assert(err, tc.ErrorIsNil)
	}
	got, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.AgentStream, tc.SameContents, seeded.AgentStream)
	c.Check(got.ChangeLogEditType, tc.SameContents, seeded.ChangeLogEditType)
	c.Check(got.ChangeLogNamespace, tc.SameContents, seeded.ChangeLogNamespace)
	wantDevices := append(seeded.StorageVolumeDeviceType,
		v4_1_0.StorageVolumeDeviceType{ID: 17, Name: "custom-null"},
		v4_1_0.StorageVolumeDeviceType{ID: 18, Name: "custom-described", Description: new("custom device description")},
	)
	wantRoles := append(seeded.SecretRole,
		v4_1_0.SecretRole{ID: 17},
		v4_1_0.SecretRole{ID: 18, Role: new("custom-role")},
	)
	c.Check(got.StorageVolumeDeviceType, tc.SameContents, wantDevices)
	c.Check(got.SecretRole, tc.SameContents, wantRoles)

	var stored int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM storage_volume_device_type
WHERE typeof(id) = 'integer'
AND ((id = 17 AND name = 'custom-null' AND description IS NULL)
OR (id = 18 AND name = 'custom-described' AND description = 'custom device description'))
`).Scan(&stored)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(stored, tc.Equals, 2)

	data, err = yaml.Marshal(got)
	c.Assert(err, tc.ErrorIsNil)
	redecoded, err := export.DecodePayload(semversion.MustParse("4.1.0"), data)
	c.Assert(err, tc.ErrorIsNil)
	roundTrip, ok := redecoded.(v4_1_0.ModelExport)
	c.Assert(ok, tc.IsTrue)
	c.Check(roundTrip.AgentStream, tc.SameContents, seeded.AgentStream)
	c.Check(roundTrip.StorageVolumeDeviceType, tc.SameContents, wantDevices)
	c.Check(roundTrip.SecretRole, tc.SameContents, wantRoles)
}

func (s *importSuite) TestLegacyLookupMissingWireID(c *tc.C) {
	for _, missing := range []string{"", "    id: null\n"} {
		data := []byte(fmt.Sprintf("agent_stream:\n  - id: 0\n    name: released\n  - name: invalid\n%s", missing))
		decoded, err := export.DecodePayload(semversion.MustParse("4.0.12"), data)
		c.Assert(err, tc.ErrorIsNil)
		transform, err := modelimport.NewTransformer()
		c.Assert(err, tc.ErrorIsNil)
		_, err = transform.Transform(c.Context(), semversion.MustParse("4.0.12"), decoded)
		c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
		c.Check(err, tc.ErrorMatches, ".*agent_stream.*row 1.*id is NULL.*")
	}
}
