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
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/agentpassword"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalpassword "github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/uuid"
)

type modelStateSuite struct {
	schematesting.ModelSuite
}

func TestModelStateSuite(t *testing.T) {
	tc.Run(t, &modelStateSuite{})
}

func (s *modelStateSuite) TestSetUnitPassword(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
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

func (s *modelStateSuite) TestSetUnitPasswordUnitDoesNotExist(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	_, err := st.GetUnitUUID(c.Context(), unit.Name("foo/0"))
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *modelStateSuite) TestSetUnitPasswordUnitNotFound(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	err := st.SetUnitPasswordHash(c.Context(), unit.UUID("foo"), passwordHash)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *modelStateSuite) TestMatchesUnitPasswordHash(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
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

func (s *modelStateSuite) TestMatchesUnitPasswordHashUnitNotFound(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	_, err := st.MatchesUnitPasswordHash(c.Context(), unit.UUID("foo"), passwordHash)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStateSuite) TestMatchesUnitPasswordHashInvalidPassword(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
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

func (s *modelStateSuite) TestGetAllUnitPasswordHashes(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
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

func (s *modelStateSuite) TestGetAllUnitPasswordHashesPasswordNotSet(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
	s.createUnit(c)

	hashes, err := st.GetAllUnitPasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.UnitPasswordHashes{
		"foo/0": "",
	})
}

func (s *modelStateSuite) TestGetAllUnitPasswordHashesNoUnits(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	hashes, err := st.GetAllUnitPasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.UnitPasswordHashes{})
}

// TestSetModelPassword tests that the model password hash can be set.
func (s *modelStateSuite) TestSetModelPassword(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())
	s.createModel(c)

	passwordHash := s.genPasswordHash(c)
	err := st.SetModelPasswordHash(c.Context(), passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the password hash was set correctly.
	var hash string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM model_agent").Scan(&hash)
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, string(passwordHash))
}

// TestMatchesModelPasswordHash tests that the model password hash can be
// matched.
func (s *modelStateSuite) TestMatchesModelPasswordHash(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())
	s.createModel(c)

	passwordHash := s.genPasswordHash(c)
	err := st.SetModelPasswordHash(c.Context(), passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesModelPasswordHash(c.Context(), passwordHash)
	c.Check(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsTrue)
}

// TestMatchesModelPasswordHashNotSet asserts that if no model password is set
// then a call to [State.MatchesModelPasswordHash] will return false with no
// error.
func (s *modelStateSuite) TestMatchesModelPasswordHashNotSet(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)
	valid, err := st.MatchesModelPasswordHash(c.Context(), passwordHash)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsFalse)
}

// TestMatchesModelPasswordHashInvalidPassword asserts that matching the
// model's password hash with an incorrect value returns false with no error.
func (s *modelStateSuite) TestMatchesModelPasswordHashInvalidPassword(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())
	s.createModel(c)

	passwordHash := s.genPasswordHash(c)
	err := st.SetModelPasswordHash(c.Context(), passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesModelPasswordHash(c.Context(), passwordHash+"1")
	c.Check(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsFalse)
}

// TestCannotHaveTwoModels asserts that the model table does not allow more
// than one model record to be created. This is because this state layer assumes
// that a maximum of only one record can exist in the model_agent table. If this
// test fails it means the model_agent table can now have more than one record.
//
// The assumption is based off of a foreign key to the model table and the model
// table being restricted to a single row. If this test fails a mistake has been
// made in the DDL or [State.MatchesModelPasswordHash] needs to be updated to
// handle this case safely.
func (s *modelStateSuite) TestCannotHaveTwoModels(c *tc.C) {
	modelUUID1 := coremodel.GenUUID(c)
	modelUUID2 := coremodel.GenUUID(c)
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test-model", "test-qualifier", "iaas", "test-cloud", "test-cloud-type")
`,
		modelUUID1.String(), controllerUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test-model", "test-qualifier", "iaas", "test-cloud", "test-cloud-type")
`,
		modelUUID2.String(), controllerUUID.String(),
	)
	c.Assert(database.IsErrConstraintUnique(err), tc.IsTrue)
}

func (s *modelStateSuite) TestSetMachinePassword(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
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

func (s *modelStateSuite) TestSetMachinePasswordMachineDoesNotExist(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	_, err := st.GetMachineUUID(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *modelStateSuite) TestSetMachinePasswordMachineNotFound(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	err := st.SetMachinePasswordHash(c.Context(), machine.UUID("foo"), passwordHash)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *modelStateSuite) TestMatchesMachinePasswordHashWithNonce(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
	machineName, nonce := s.createMachine(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE machine_cloud_instance
SET instance_id = 'abc'
WHERE machine_uuid = (
    SELECT uuid FROM machine WHERE name = ?);
`, machineName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	machineUUID, err := st.GetMachineUUID(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetMachinePasswordHash(c.Context(), machineUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesMachinePasswordHashWithNonce(c.Context(), machineUUID, passwordHash, nonce)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsTrue)
}

func (s *modelStateSuite) TestMatchesMachinePasswordHashWithNonceMachineNotFound(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	_, err := st.MatchesMachinePasswordHashWithNonce(c.Context(), machine.UUID("foo"), passwordHash, "")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStateSuite) TestMatchesMachinePasswordHashWithNonceInvalidPassword(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
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

func (s *modelStateSuite) TestMatchesMachinePasswordHashWithNonceNotProvisioned(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
	machineName, nonce := s.createMachine(c)

	machineUUID, err := st.GetMachineUUID(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetMachinePasswordHash(c.Context(), machineUUID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesMachinePasswordHashWithNonce(c.Context(), machineUUID, passwordHash, nonce)
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
	c.Check(valid, tc.IsFalse)
}

func (s *modelStateSuite) TestGetAllMachinePasswordHashes(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
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

func (s *modelStateSuite) TestGetAllMachinePasswordHashesPasswordNotSet(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	s.createApplication(c, false)
	s.createMachine(c)

	hashes, err := st.GetAllMachinePasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.MachinePasswordHashes{
		"0": "",
	})
}

func (s *modelStateSuite) TestGetAllMachinePasswordHashesNoMachines(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	hashes, err := st.GetAllMachinePasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hashes, tc.DeepEquals, agentpassword.MachinePasswordHashes{})
}

func (s *modelStateSuite) TestIsMachineControllerApplicationController(c *tc.C) {
	s.createApplication(c, true)

	st := NewModelState(s.TxnRunnerFactory())

	machineName, _ := s.createMachine(c)

	isController, err := st.IsMachineController(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsTrue)
}

func (s *modelStateSuite) TestIsMachineControllerApplicationNonController(c *tc.C) {
	s.createApplication(c, false)

	st := NewModelState(s.TxnRunnerFactory())

	machineName, _ := s.createMachine(c)

	isController, err := st.IsMachineController(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsFalse)
}

func (s *modelStateSuite) TestIsMachineControllerFailure(c *tc.C) {
	s.createApplication(c, false)

	st := NewModelState(s.TxnRunnerFactory())

	machineName, _ := s.createMachine(c)

	isController, err := st.IsMachineController(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsFalse)
}

// TestIsMachineControllerNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *modelStateSuite) TestIsMachineControllerNotFound(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	_, err := st.IsMachineController(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestSetApplicationPassword asserts that an application password hash is set to
// the expected hash value.
func (s *modelStateSuite) TestSetApplicationPassword(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	appID := s.createApplication(c, false)

	passwordHash := s.genPasswordHash(c)

	err := st.SetApplicationPasswordHash(c.Context(), appID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the password hash was set correctly.
	var hash string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM application_agent WHERE application_uuid = ?", appID).Scan(&hash)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hash, tc.Equals, string(passwordHash))
}

// TestGetApplicationIDByName asserts that an application ID can be found by name.
func (s *modelStateSuite) TestGetApplicationIDByName(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	appID := s.createApplication(c, false)

	gotAppID, err := st.GetApplicationIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotAppID, tc.Equals, appID)
}

// TestGetApplicationIDByNameNotFound asserts that an application not found error
// is returned when the named application cannot be found.
func (s *modelStateSuite) TestGetApplicationIDByNameNotFound(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	_, err := st.GetApplicationIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

// TestSetApplicationPasswordApplicationNotFound asserts that an application not
// found error is returned when the identified application does not exist when
// the password is set.
func (s *modelStateSuite) TestSetApplicationPasswordApplicationNotFound(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	appID, err := coreapplication.NewID()
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetApplicationPasswordHash(c.Context(), appID, passwordHash)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

// TestMatchesApplicationPasswordHash asserts that the provided password hash
// matches the hash stored for the identified application.
func (s *modelStateSuite) TestMatchesApplicationPasswordHash(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	appID := s.createApplication(c, false)

	passwordHash := s.genPasswordHash(c)

	err := st.SetApplicationPasswordHash(c.Context(), appID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesApplicationPasswordHash(c.Context(), appID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valid, tc.IsTrue)
}

// TestMatchesApplicationPasswordHashApplicationNotFound asserts that the
// password hash does not match for the missing identified application but no
// error is returned.
func (s *modelStateSuite) TestMatchesApplicationPasswordHashApplicationNotFound(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	appID, err := coreapplication.NewID()
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	valid, err := st.MatchesApplicationPasswordHash(c.Context(), appID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valid, tc.IsFalse)
}

// TestMatchesApplicationPasswordHashInvalidPassword asserts that the provided
// password hash does not match the stored password hash for the identified app.
func (s *modelStateSuite) TestMatchesApplicationPasswordHashInvalidPassword(c *tc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	appID := s.createApplication(c, false)

	passwordHash := s.genPasswordHash(c)

	err := st.SetApplicationPasswordHash(c.Context(), appID, passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesApplicationPasswordHash(c.Context(), appID, passwordHash+"1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valid, tc.IsFalse)
}

func (s *modelStateSuite) genPasswordHash(c *tc.C) agentpassword.PasswordHash {
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	return agentpassword.PasswordHash(internalpassword.AgentPasswordHash(rand))
}

func (s *modelStateSuite) createApplication(c *tc.C, controller bool) coreapplication.ID {
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
			IsController: controller,
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	return appID
}

// createModel is testing utility function for establishing the readonly model
// information in the database along with a record for the model in the
// model_agent table.
func (s *modelStateSuite) createModel(c *tc.C) {
	modelUUID := coremodel.GenUUID(c)
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test-model", "test-qualifier", "iaas", "test-cloud", "test-cloud-type")
`,
		modelUUID.String(), controllerUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO model_agent (model_uuid) VALUES (?)
`,
		modelUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStateSuite) createUnit(c *tc.C) unit.Name {
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

func (s *modelStateSuite) createMachine(c *tc.C) (machine.Name, string) {
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
