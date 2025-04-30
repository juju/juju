// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/domain/modelagent"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite

	machineName coremachine.Name
}

var _ = gc.Suite(&stateSuite{})

var (
	testingPublicKeys = []string{
		// ecdsa testing public key
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju1@example.com",

		// ed25519 testing public key
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju2@example.com",

		// rsa testing public key
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju3@example.com",
	}
)

// ensureNetNode inserts a row into the net_node table, mostly used as a foreign key for entries in
// other tables (e.g. machine)
func (s *stateSuite) ensureNetNode(c *gc.C, uuid string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO net_node (uuid)
			VALUES (?)`, uuid)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) ensureMachine(c *gc.C, name coremachine.Name, uuid string) {
	s.ensureNetNode(c, "node2")
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO machine (uuid, net_node_uuid, name, life_id)
		VALUES (?, "node2", ?, "0")`, uuid, name)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.machineName = coremachine.Name("0")
	s.ensureMachine(c, s.machineName, "123")
}

// TestCheckMachineExists is asserting the happy path of
// [State.CheckMachineExists] and that if a machine that exists is asked for no
// error is returned.
func (s *stateSuite) TestCheckMachineExists(c *gc.C) {
	err := NewState(s.TxnRunnerFactory()).CheckMachineExists(
		context.Background(),
		s.machineName,
	)
	c.Check(err, jc.ErrorIsNil)
}

// TestCheckMachineDoesNotExist is asserting the if we ask for a machine that
// doesn't exist we get back [machineerrors.MachineNotFound] error.
func (s *stateSuite) TestCheckMachineDoesNotExist(c *gc.C) {
	err := NewState(s.TxnRunnerFactory()).CheckMachineExists(
		context.Background(),
		coremachine.Name("100"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestGetModelId(c *gc.C) {
	mst := modelstate.NewModelState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            modelUUID,
		AgentVersion:    jujuversion.Current,
		AgentStream:     modelagent.AgentStreamReleased,
		ControllerUUID:  uuid.MustNewUUID(),
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := mst.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	rval, err := NewState(s.TxnRunnerFactory()).GetModelUUID(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, modelUUID)
}
