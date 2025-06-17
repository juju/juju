// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/agentpassword"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalpassword "github.com/juju/juju/internal/password"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestSetUnitPassword(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	unitName := s.createUnit(c)

	unitUUID, err := st.GetUnitUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetUnitPasswordHash(c.Context(), unitUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the password hash was set correctly.
	var hash string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM unit WHERE uuid = ?", unitUUID).Scan(&hash)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hash, tc.Equals, string(passwordHash))
}

func (s *stateSuite) TestSetUnitPasswordUnitDoesNotExist(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetUnitUUID(c.Context(), unit.Name("foo/0"))
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestSetUnitPasswordUnitNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	err := st.SetUnitPasswordHash(c.Context(), unit.UUID("foo"), passwordHash)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestMatchesUnitPasswordHash(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	unitName := s.createUnit(c)

	unitUUID, err := st.GetUnitUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetUnitPasswordHash(c.Context(), unitUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesUnitPasswordHash(c.Context(), unitUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valid, tc.IsTrue)
}

func (s *stateSuite) TestMatchesUnitPasswordHashUnitNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	_, err := st.MatchesUnitPasswordHash(c.Context(), unit.UUID("foo"), passwordHash)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestMatchesUnitPasswordHashInvalidPassword(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	unitName := s.createUnit(c)

	unitUUID, err := st.GetUnitUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetUnitPasswordHash(c.Context(), unitUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesUnitPasswordHash(c.Context(), unitUUID, passwordHash+"1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valid, tc.IsFalse)
}

func (s *stateSuite) TestGetAllUnitPasswordHashes(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	unitName := s.createUnit(c)

	unitUUID, err := st.GetUnitUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetUnitPasswordHash(c.Context(), unitUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	hashes, err := st.GetAllUnitPasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.UnitPasswordHashes{
		unitName: passwordHash,
	})
}

func (s *stateSuite) TestGetAllUnitPasswordHashesPasswordNotSet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	s.createUnit(c)

	hashes, err := st.GetAllUnitPasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.UnitPasswordHashes{
		"foo/0": "",
	})
}

func (s *stateSuite) TestGetAllUnitPasswordHashesNoUnits(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	hashes, err := st.GetAllUnitPasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.UnitPasswordHashes{})
}

func (s *stateSuite) TestSetMachinePassword(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	machineName, _ := s.createMachine(c)

	machineUUID, err := st.GetMachineUUID(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetMachinePasswordHash(c.Context(), machineUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the password hash was set correctly.
	var hash string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM machine WHERE uuid = ?", machineUUID).Scan(&hash)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hash, tc.Equals, string(passwordHash))
}

func (s *stateSuite) TestSetMachinePasswordMachineDoesNotExist(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetMachineUUID(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetMachinePasswordMachineNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	err := st.SetMachinePasswordHash(c.Context(), machine.UUID("foo"), passwordHash)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestMatchesMachinePasswordHash(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	machineName, nonce := s.createMachine(c)

	machineUUID, err := st.GetMachineUUID(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetMachinePasswordHash(c.Context(), machineUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesMachinePasswordHashWithNonce(c.Context(), machineUUID, passwordHash, nonce)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsTrue)
}

func (s *stateSuite) TestMatchesMachinePasswordHashMachineNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	_, err := st.MatchesMachinePasswordHashWithNonce(c.Context(), machine.UUID("foo"), passwordHash, "")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestMatchesMachinePasswordHashInvalidPassword(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	machineName, nonce := s.createMachine(c)

	machineUUID, err := st.GetMachineUUID(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetMachinePasswordHash(c.Context(), machineUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesMachinePasswordHashWithNonce(c.Context(), machineUUID, passwordHash+"1", nonce)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsFalse)
}

func (s *stateSuite) TestGetAllMachinePasswordHashes(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	machineName, _ := s.createMachine(c)

	machineUUID, err := st.GetMachineUUID(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetMachinePasswordHash(c.Context(), machineUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	hashes, err := st.GetAllMachinePasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.MachinePasswordHashes{
		machineName: passwordHash,
	})
}

func (s *stateSuite) TestGetAllMachinePasswordHashesPasswordNotSet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	s.createMachine(c)

	hashes, err := st.GetAllMachinePasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.MachinePasswordHashes{
		"0": "",
	})
}

func (s *stateSuite) TestGetAllMachinePasswordHashesNoMachines(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	hashes, err := st.GetAllMachinePasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.MachinePasswordHashes{})
}

func (s *stateSuite) genPasswordHash(c *tc.C) agentpassword.PasswordHash {
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	return agentpassword.PasswordHash(internalpassword.AgentPasswordHash(rand))
}

func (s *stateSuite) createApplication(c *tc.C) coreapplication.ID {
	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	appID, _, err := applicationSt.CreateIAASApplication(c.Context(), "foo", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "foo",
				},
				Manifest: charm.Manifest{
					Bases: []charm.Base{{
						Name:          "ubuntu",
						Channel:       charm.Channel{Risk: charm.RiskStable},
						Architectures: []string{"amd64"},
					}},
				},
				ReferenceName: "foo",
				Architecture:  architecture.AMD64,
				Revision:      1,
				Source:        charm.LocalSource,
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	return appID
}

func (s *stateSuite) createUnit(c *tc.C) unit.Name {
	ctx := c.Context()
	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	appID, err := applicationSt.GetApplicationIDByName(ctx, "foo")
	c.Assert(err, tc.ErrorIsNil)

	unitNames, _, err := applicationSt.AddIAASUnits(ctx, appID, application.AddIAASUnitArg{
		Nonce: ptr("foo"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.HasLen, 1)
	unitName := unitNames[0]

	return unitName
}

func (s *stateSuite) createMachine(c *tc.C) (machine.Name, string) {
	unitName := s.createUnit(c)

	var machineName machine.Name
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT m.name
FROM machine m
JOIN net_node nn ON m.net_node_uuid = nn.uuid
JOIN unit u ON u.net_node_uuid = nn.uuid
WHERE u.name = ?		
`, unitName).Scan(&machineName)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return machineName, "foo"
}

func ptr[T any](v T) *T {
	return &v
}
