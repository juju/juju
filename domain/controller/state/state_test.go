// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	controllererrors "github.com/juju/juju/domain/controller/errors"
	"github.com/juju/juju/domain/controllernode"
	controllernodestate "github.com/juju/juju/domain/controllernode/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
	controllerModelUUID coremodel.UUID
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.controllerModelUUID = coremodel.UUID(jujutesting.ModelTag.Id())
	s.ControllerSuite.SetUpTest(c)
	_ = s.SeedControllerTable(c, s.controllerModelUUID)
}

func (s *stateSuite) TestControllerModelUUID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	uuid, err := st.GetControllerModelUUID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, s.controllerModelUUID)
}

func (s *stateSuite) TestGetControllerAgentInfo(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	controllerAgentInfo, err := st.GetControllerAgentInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(controllerAgentInfo, tc.DeepEquals, controller.ControllerAgentInfo{
		APIPort:        17070,
		Cert:           "test-cert",
		PrivateKey:     "test-private-key",
		CAPrivateKey:   "test-ca-private-key",
		SystemIdentity: "test-system-identity",
	})
}

func (s *stateSuite) TestGetControllerAgentInfoNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM controller")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetControllerAgentInfo(c.Context())
	c.Assert(err, tc.ErrorIs, controllererrors.NotFound)
}

func (s *stateSuite) TestGetModelNamespacesNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	allNamespaces, err := st.GetModelNamespaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allNamespaces, tc.DeepEquals, []string{})
}

func (s *stateSuite) TestGetModelNamespaces(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO namespace_list (namespace) VALUES ('namespace1'), ('namespace2')")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	allNamespaces, err := st.GetModelNamespaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allNamespaces, tc.DeepEquals, []string{"namespace1", "namespace2"})
}

func (s *stateSuite) TestGetCACert(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	cert, err := st.GetCACert(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cert, tc.Equals, "test-ca-cert")
}

func (s *stateSuite) TestGetControllerInfo(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	controllerNodeState := controllernodestate.NewState(s.TxnRunnerFactory())
	// Arrange: 2 controller nodes
	controllerID1 := "1"
	nodeID1 := uint64(15237855465837235027)
	err := controllerNodeState.AddDqliteNode(c.Context(), controllerID1, nodeID1, "10.0.0."+controllerID1)
	c.Assert(err, tc.ErrorIsNil)

	addrs1 := []controllernode.APIAddress{
		{Address: "10.0.0.2:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.42:18080", IsAgent: true, Scope: network.ScopePublic},
		{Address: "192.168.0.1:17070", IsAgent: false, Scope: network.ScopeMachineLocal},
	}
	err = controllerNodeState.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID1: addrs1,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.UUID, tc.Equals, "deadbeef-1bad-500d-9000-4b1d0d06f00d")
	c.Check(info.CACert, tc.Equals, "test-ca-cert")
	c.Check(info.APIAddresses, tc.SameContents, []string{"10.0.0.2:17070", "10.0.0.42:18080"})
}
