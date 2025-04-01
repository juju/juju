// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	passworderrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/password"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalpassword "github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestSetUnitPassword(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	unitName := s.createUnit(c, 0)

	unitUUID, err := st.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetUnitPasswordHash(context.Background(), unitUUID, passwordHash)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the password hash was set correctly.
	var hash string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM unit WHERE uuid = ?", unitUUID).Scan(&hash)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hash, gc.Equals, string(passwordHash))
}

func (s *stateSuite) TestSetUnitPasswordUnitDoesNotExist(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetUnitUUID(context.Background(), unit.Name("foo/0"))
	c.Assert(err, jc.ErrorIs, passworderrors.UnitNotFound)
}

func (s *stateSuite) TestSetUnitPasswordUnitNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	err := st.SetUnitPasswordHash(context.Background(), unit.UUID("foo"), passwordHash)
	c.Assert(err, jc.ErrorIs, passworderrors.UnitNotFound)
}

func (s *stateSuite) TestIsValidUnitPassword(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	unitName := s.createUnit(c, 0)

	unitUUID, err := st.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetUnitPasswordHash(context.Background(), unitUUID, passwordHash)
	c.Assert(err, jc.ErrorIsNil)

	valid, err := st.IsValidUnitPassword(context.Background(), unitUUID, passwordHash)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valid, jc.IsTrue)
}

func (s *stateSuite) TestIsValidUnitPasswordUnitNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	_, err := st.IsValidUnitPassword(context.Background(), unit.UUID("foo"), passwordHash)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestIsValidUnitPasswordInvalidPassword(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	unitName := s.createUnit(c, 0)

	unitUUID, err := st.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetUnitPasswordHash(context.Background(), unitUUID, passwordHash)
	c.Assert(err, jc.ErrorIsNil)

	valid, err := st.IsValidUnitPassword(context.Background(), unitUUID, passwordHash+"1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valid, jc.IsFalse)
}

func (s *stateSuite) TestGetAllUnitPasswordHashes(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	unitName := s.createUnit(c, 0)

	unitUUID, err := st.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)

	passwordHash := s.genPasswordHash(c)

	err = st.SetUnitPasswordHash(context.Background(), unitUUID, passwordHash)
	c.Assert(err, jc.ErrorIsNil)

	hashes, err := st.GetAllUnitPasswordHashes(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hashes, jc.DeepEquals, password.UnitPasswordHashes{
		unitName: passwordHash,
	})
}

func (s *stateSuite) TestGetAllUnitPasswordHashesPasswordNotSet(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.createApplication(c)
	s.createUnit(c, 0)

	hashes, err := st.GetAllUnitPasswordHashes(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hashes, jc.DeepEquals, password.UnitPasswordHashes{
		"foo/0": "",
	})
}

func (s *stateSuite) TestGetAllUnitPasswordHashesNoUnits(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	hashes, err := st.GetAllUnitPasswordHashes(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hashes, jc.DeepEquals, password.UnitPasswordHashes{})
}

func (s *stateSuite) genPasswordHash(c *gc.C) password.PasswordHash {
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	return password.PasswordHash(internalpassword.AgentPasswordHash(rand))
}

func (s *stateSuite) createApplication(c *gc.C) {
	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	_, err := applicationSt.CreateApplication(context.Background(), "foo", application.AddApplicationArg{
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
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) createUnit(c *gc.C, num int) unit.Name {
	netNodeUUID := uuid.MustNewUUID().String()

	unitName, err := unit.NewNameFromParts("foo", num)
	c.Assert(err, jc.ErrorIsNil)
	ctx := context.Background()
	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	appID, err := applicationSt.GetApplicationIDByName(ctx, "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = applicationSt.AddIAASUnits(ctx, c.MkDir(), appID, application.AddUnitArg{UnitName: unitName})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err = tx.ExecContext(ctx, "INSERT INTO net_node VALUES (?) ON CONFLICT DO NOTHING", netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "UPDATE unit SET net_node_uuid = ? WHERE name = ?", netNodeUUID, unitName)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return unitName
}
