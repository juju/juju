// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/juju/tc"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
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

// addControllerNodeAgentVersion adds a controller node to the cluster and sets
// its current reported agent version to the given value. The new controller
// node id is returned.
func (s *controllerStateSuite) addControllerNodeAgentVersion(
	c *tc.C, version string,
) string {
	id, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(
		"INSERT INTO controller_node (controller_id) VALUES (?)", id.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO controller_node_agent_version (controller_id, version, architecture_id)
VALUES (?, ?, ?)
`,
		id.String(), version, 0,
	)
	c.Assert(err, tc.ErrorIsNil)

	return id.String()
}

// setInitialControllerTargetVersion establishes a controller uuid and sets the
// initial target version of the controller.
func (s *controllerStateSuite) setInitialControllerTargetVersion(
	c *tc.C, version string,
) {
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	modelUUID := modeltesting.GenModelUUID(c)

	_, err = s.DB().Exec("INSERT INTO controller (uuid, model_uuid, target_version) VALUES (?, ?, ?)", controllerUUID.String(), modelUUID.String(), version)
	c.Assert(err, tc.ErrorIsNil)
}

// addObjectStore inserts a new row to `object_store_metadata` table. Its UUID is returned.
func (s *controllerStateSuite) addObjectStore(c *tc.C) objectstore.UUID {
	storeUUID := tc.Must(c, objectstore.NewUUID)
	hasher256 := sha256.New()
	hasher384 := sha512.New384()
	_, err := io.Copy(io.MultiWriter(hasher256, hasher384), strings.NewReader(storeUUID.String()))
	c.Assert(err, tc.ErrorIsNil)
	sha256Hash := hex.EncodeToString(hasher256.Sum(nil))
	sha384Hash := hex.EncodeToString(hasher384.Sum(nil))

	_, err = s.DB().Exec(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES (?, ?, ?, ?)
`, storeUUID, sha256Hash, sha384Hash, 1234)
	c.Assert(err, tc.ErrorIsNil)

	return storeUUID
}

// addAgentBinaryStore inserts a new row to `agent_binary_store` table.
// It is dependent upon architecture and object store metadata for its foreign keys.
// Architecture is auto seeded in the DDL. However, addObjectStore must be invoked prior to
// addAgentBinaryStore.
func (s *controllerStateSuite) addAgentBinaryStore(c *tc.C, version semversion.Number, architecture domainagentbinary.Architecture, storeUUID objectstore.UUID) {
	_, err := s.DB().Exec(`
INSERT INTO agent_binary_store(version, architecture_id, object_store_uuid) VALUES(?, ?, ?)
`, version.String(), int(architecture), storeUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetControllerNodeVersionsEmpty tests that when no controller node
// versions have been reported an empty value is returned with no error.
func (s *controllerStateSuite) TestGetControllerNodeVersionsEmpty(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())
	versions, err := st.GetControllerNodeVersions(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(versions, tc.HasLen, 0)
}

// TestGetControllerNodeVersions verifies that the controller node versions are
// reported correctly when two nodes have their version recorded.
func (s *controllerStateSuite) TestGetControllerNodeVersions(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())

	c1Version, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	id1 := s.addControllerNodeAgentVersion(c, c1Version.String())
	c2Version, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)
	id2 := s.addControllerNodeAgentVersion(c, c2Version.String())

	// Get the versions.
	versions, err := st.GetControllerNodeVersions(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(versions, tc.DeepEquals, map[string]semversion.Number{
		id1: c1Version,
		id2: c2Version,
	})
}

// TestSetAndGetControllerVersion tests that the controller version can be
// retrieved with no errors and can also be set (upgraded) with no errors.
// This is a happy path test of:
// - [ControllerState.GetControllerVersion]
// - [ControllerState.SetControllerTargetVersion]
func (s *controllerStateSuite) TestSetAndGetControllerVersion(c *tc.C) {
	initialVersion, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	upgradeVersion, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)

	s.setInitialControllerTargetVersion(c, initialVersion.String())
	st := NewControllerState(s.TxnRunnerFactory())

	// Check initial version is reported correctly.
	ver, err := st.GetControllerTargetVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.Equals, initialVersion)

	// Upgrade version.
	err = st.SetControllerTargetVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)

	// Check upgraded version is reported correctly.
	ver, err = st.GetControllerTargetVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.Equals, upgradeVersion)
}

// TestSetControllerVersionMultipleSetSafe tests that setting the controller
// target version multiple times to the same value is safe and doesn't produce
// an error.
//
// This is a requirement of the service layer caller neededing to be able to get
// state back in a consistent state.
func (s *controllerStateSuite) TestSetControllerVersionMultipleSetSafe(c *tc.C) {
	initialVersion, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	upgradeVersion, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)

	s.setInitialControllerTargetVersion(c, initialVersion.String())
	st := NewControllerState(s.TxnRunnerFactory())

	// Upgrade version #1.
	err = st.SetControllerTargetVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)

	// Upgrade version #2.
	err = st.SetControllerTargetVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)

	// Upgrade version #3.
	err = st.SetControllerTargetVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)

	// Check upgraded version is reported correctly.
	ver, err := st.GetControllerTargetVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.Equals, upgradeVersion)
}

// TestHasAgentBinaryForVersionArchitecturesAndStream tests determining whether an agent for
// a given version and architectures work without errors.
func (s *controllerStateSuite) TestHasAgentBinaryForVersionArchitecturesAndStream(c *tc.C) {
	version, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	storeUUID := s.addObjectStore(c)
	s.addAgentBinaryStore(c, version, domainagentbinary.AMD64, storeUUID)
	s.addAgentBinaryStore(c, version, domainagentbinary.ARM64, storeUUID)

	st := NewControllerState(s.TxnRunnerFactory())

	agents, err := st.HasAgentBinariesForVersionArchitecturesAndStream(c.Context(), version, []domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.ARM64}, domainagentbinary.AgentStreamReleased)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agents, tc.DeepEquals, map[domainagentbinary.Architecture]bool{
		domainagentbinary.AMD64: true,
		domainagentbinary.ARM64: true,
	})
}

// TestHasAgentBinaryForVersionArchitecturesAndStream tests determining whether an agent for
// a given version and architectures work with some architectures not existing.
func (s *controllerStateSuite) TestHasAgentBinaryForVersionArchitecturesAndStreamNotSupported(c *tc.C) {
	version, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	storeUUID := s.addObjectStore(c)
	s.addAgentBinaryStore(c, version, domainagentbinary.AMD64, storeUUID)
	s.addAgentBinaryStore(c, version, domainagentbinary.ARM64, storeUUID)

	st := NewControllerState(s.TxnRunnerFactory())

	agents, err := st.HasAgentBinariesForVersionArchitecturesAndStream(c.Context(), version, []domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.PPC64EL, domainagentbinary.RISCV64}, domainagentbinary.AgentStreamReleased)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agents, tc.DeepEquals, map[domainagentbinary.Architecture]bool{
		domainagentbinary.AMD64:   true,
		domainagentbinary.PPC64EL: false,
		domainagentbinary.RISCV64: false,
	})
}
