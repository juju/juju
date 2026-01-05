// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	agentbinarystate "github.com/juju/juju/domain/agentbinary/state"
	"github.com/juju/juju/domain/application/architecture"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

// controllerStateSuite is a collection of tests to assert the contracts of
// offered by [ControllerState].
type controllerStateSuite struct {
	schematesting.ControllerSuite
}

// TestControllerStateSuite runs the tests in [controllerStateSuite].
func TestControllerStateSuite(t *testing.T) {
	tc.Run(t, &controllerStateSuite{})
}

// seedControllersAndAgents inserts values to the controller_node and
// controller_node_agent_version that will be used to query the versions.
func (s *controllerStateSuite) seedControllersAndAgents(c *tc.C) {
	res, err := s.DB().Exec(`
INSERT INTO controller_node(controller_id, dqlite_node_id)
VALUES ('1', '1'), 
       ('2', '2'), 
       ('3', '3'),
       ('4', '4')`)

	c.Assert(err, tc.ErrorIsNil)

	rows, err := res.RowsAffected()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows, tc.Equals, int64(4))

	// The architecture_id for amd64 is 0.
	res, err = s.DB().Exec(`
INSERT INTO controller_node_agent_version (controller_id, version, architecture_id)
VALUES (1, '4.0.1', 0), (2, '4.0.2', 0), (3, '4.0.3', 0), (4, '4.0.3', 0)`)
	c.Assert(err, tc.ErrorIsNil)

	rows, err = res.RowsAffected()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows, tc.Equals, int64(4))
}

// TestGetControllerAgentVersions tests that we can get the
// agent versions running on all controllers. It also tests that the GROUP BY
// clause works as intended.
func (s *controllerStateSuite) TestGetControllerAgentVersions(c *tc.C) {
	s.seedControllersAndAgents(c)
	st := NewState(s.TxnRunnerFactory())

	versions, err := st.GetControllerAgentVersions(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	version1, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)
	version2, err := semversion.Parse("4.0.2")
	c.Assert(err, tc.ErrorIsNil)
	version3, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)

	expected := []semversion.Number{
		version1,
		version2,
		version3,
	}
	c.Check(versions, tc.SameContents, expected)
}

// TestGetControllerAgentVersionsNoneFound tests a sad case
// that controller agents are not found.
func (s *controllerStateSuite) TestGetControllerAgentVersionsNoneFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	versions, err := st.GetControllerAgentVersions(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Check(versions, tc.HasLen, 0)
}

func (s *controllerStateSuite) TestGetMissingMachineTargetAgentVersionByArchesNoRecordedArches(c *tc.C) {
	expectedVersion, err := semversion.Parse("4.21.54")
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())

	missing, err := st.GetMissingMachineTargetAgentVersionByArches(c.Context(), expectedVersion.String(), map[architecture.Architecture]struct{}{
		architecture.AMD64: {},
		architecture.ARM64: {},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(missing, tc.HasLen, 2)
	c.Check(missing, tc.DeepEquals, map[architecture.Architecture]struct{}{
		architecture.AMD64: {},
		architecture.ARM64: {},
	})
}

func (s *controllerStateSuite) TestGetMissingMachineTargetAgentVersionByArches(c *tc.C) {
	expectedVersion, err := semversion.Parse("4.21.54")
	c.Assert(err, tc.ErrorIsNil)

	versionAMD64 := coreagentbinary.Version{
		Number: expectedVersion,
		Arch:   corearch.AMD64,
	}
	versionARM64 := coreagentbinary.Version{
		Number: expectedVersion,
		Arch:   corearch.ARM64,
	}

	s.registerAgentBinary(c, versionAMD64)
	s.registerAgentBinary(c, versionARM64)

	st := NewState(s.TxnRunnerFactory())

	missing, err := st.GetMissingMachineTargetAgentVersionByArches(c.Context(), expectedVersion.String(), map[architecture.Architecture]struct{}{
		architecture.AMD64: {},
		architecture.ARM64: {},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(missing, tc.HasLen, 0)
}

func (s *controllerStateSuite) TestGetMissingMachineTargetAgentVersionByArchesMultipleVersions(c *tc.C) {
	expectedVersion, err := semversion.Parse("4.21.54")
	c.Assert(err, tc.ErrorIsNil)

	versionAMD64 := coreagentbinary.Version{
		Number: expectedVersion,
		Arch:   corearch.AMD64,
	}
	versionAMD64Alt1 := coreagentbinary.Version{
		Number: semversion.MustParse("4.21.52"),
		Arch:   corearch.AMD64,
	}
	versionAMD64Alt2 := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.54"),
		Arch:   corearch.AMD64,
	}
	versionARM64 := coreagentbinary.Version{
		Number: expectedVersion,
		Arch:   corearch.ARM64,
	}

	s.registerAgentBinary(c, versionAMD64)
	s.registerAgentBinary(c, versionAMD64Alt1)
	s.registerAgentBinary(c, versionAMD64Alt2)
	s.registerAgentBinary(c, versionARM64)

	st := NewState(s.TxnRunnerFactory())

	missing, err := st.GetMissingMachineTargetAgentVersionByArches(c.Context(), expectedVersion.String(), map[architecture.Architecture]struct{}{
		architecture.AMD64: {},
		architecture.ARM64: {},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(missing, tc.HasLen, 0)
}

func (s *controllerStateSuite) TestGetMissingMachineTargetAgentVersionByArchesMultipleVersionsWithDifferentArches(c *tc.C) {
	expectedVersion, err := semversion.Parse("4.21.54")
	c.Assert(err, tc.ErrorIsNil)

	versionAMD64 := coreagentbinary.Version{
		Number: expectedVersion,
		Arch:   corearch.AMD64,
	}
	versionAMD64Alt1 := coreagentbinary.Version{
		Number: semversion.MustParse("4.21.52"),
		Arch:   corearch.AMD64,
	}
	versionAMD64Alt2 := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.54"),
		Arch:   corearch.AMD64,
	}
	versionARM64 := coreagentbinary.Version{
		Number: expectedVersion,
		Arch:   corearch.ARM64,
	}

	s.registerAgentBinary(c, versionAMD64)
	s.registerAgentBinary(c, versionAMD64Alt1)
	s.registerAgentBinary(c, versionAMD64Alt2)
	s.registerAgentBinary(c, versionARM64)

	st := NewState(s.TxnRunnerFactory())

	missing, err := st.GetMissingMachineTargetAgentVersionByArches(c.Context(), expectedVersion.String(), map[architecture.Architecture]struct{}{
		architecture.AMD64:   {},
		architecture.ARM64:   {},
		architecture.PPC64EL: {},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(missing, tc.HasLen, 1)
	c.Check(missing, tc.DeepEquals, map[architecture.Architecture]struct{}{
		architecture.PPC64EL: {},
	})
}

// registerAgentBinary is a testing utility function that registers the fact
// that an agent binary exists in the models store for the provided version. The
// metadata for the newly created binary is returned to the caller upon creation.
func (s *controllerStateSuite) registerAgentBinary(
	c *tc.C,
	version coreagentbinary.Version,
) coreagentbinary.Metadata {
	runner := s.TxnRunner()

	type objectStoreMeta struct {
		UUID   string `db:"uuid"`
		SHA256 string `db:"sha_256"`
		SHA384 string `db:"sha_384"`
		Size   int    `db:"size"`
	}

	storeUUID := uuid.MustNewUUID().String()
	stmt, err := sqlair.Prepare(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES ($objectStoreMeta.uuid, $objectStoreMeta.sha_256, $objectStoreMeta.sha_384, $objectStoreMeta.size)
`, objectStoreMeta{})
	c.Assert(err, tc.ErrorIsNil)

	hasher256 := sha256.New()
	hasher384 := sha512.New384()
	_, err = io.Copy(io.MultiWriter(hasher256, hasher384), strings.NewReader(storeUUID))
	c.Assert(err, tc.ErrorIsNil)
	sha256Hash := hex.EncodeToString(hasher256.Sum(nil))
	sha384Hash := hex.EncodeToString(hasher384.Sum(nil))

	metaRecord := objectStoreMeta{
		UUID:   storeUUID,
		SHA256: sha256Hash,
		SHA384: sha384Hash,
		Size:   1234,
	}
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, metaRecord).Run()
	})
	c.Assert(err, tc.ErrorIsNil)

	type dbMetadataPath struct {
		// UUID is the uuid for the metadata.
		UUID string `db:"metadata_uuid"`
		// Path is the path to the object.
		Path string `db:"path"`
	}
	path := "/path/" + storeUUID
	pathRecord := dbMetadataPath{
		UUID: storeUUID,
		Path: path,
	}
	pathStmt, err := sqlair.Prepare(`
INSERT INTO object_store_metadata_path (path, metadata_uuid)
VALUES ($dbMetadataPath.*)`, pathRecord)
	c.Assert(err, tc.ErrorIsNil)
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, pathStmt, pathRecord).Run()
	})
	c.Assert(err, tc.ErrorIsNil)

	err = agentbinarystate.NewModelState(s.TxnRunnerFactory()).RegisterAgentBinary(
		c.Context(),
		domainagentbinary.RegisterAgentBinaryArg{
			Arch:            version.Arch,
			ObjectStoreUUID: objectstore.UUID(storeUUID),
			Version:         version.Number.String(),
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	return coreagentbinary.Metadata{
		SHA256:  sha256Hash,
		SHA384:  sha384Hash,
		Size:    1234,
		Version: version,
	}
}
