// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct{}

func TestServiceSuite(t *testing.T) { tc.Run(t, &serviceSuite{}) }

// Embedding the contract makes unexpected persistence calls fail the test.
type recordingState struct {
	State
	controller     *domainrecovery.ControllerDump
	controllerNode string
	model          *domainrecovery.ModelDump
	modelNode      string
	loadErr        error
}

func (st *recordingState) LoadControllerDump(_ context.Context, dump *domainrecovery.ControllerDump, nodeID string) error {
	st.controller = dump
	st.controllerNode = nodeID
	return st.loadErr
}

func (st *recordingState) LoadModelDump(_ context.Context, dump *domainrecovery.ModelDump, nodeID string) error {
	st.model = dump
	st.modelNode = nodeID
	return st.loadErr
}

const controllerYAML = `version: 4.1.0
payload:
  controller:
  - uuid: source
    model_uuid: model
    target_version: 4.1.0
    api_port: null
    cert: null
    ca_cert: null
    private_key: null
    ca_private_key: null
    system_identity: null
`

func (s *serviceSuite) TestRequiredDependencies(c *tc.C) {
	_, err := NewService(nil, func(context.Context, string) (State, error) { return &recordingState{}, nil }, "bootstrap-node")
	c.Assert(err, tc.ErrorMatches, "recovery requires controller state and a model state factory")
	_, err = NewService(&recordingState{}, nil, "bootstrap-node")
	c.Assert(err, tc.ErrorMatches, "recovery requires controller state and a model state factory")
}

func (s *serviceSuite) TestControllerPoliciesAndLeases(c *tc.C) {
	st := &recordingState{}
	svc, err := NewService(st, func(context.Context, string) (State, error) { return st, nil }, "bootstrap-node")
	c.Assert(err, tc.ErrorIsNil)
	data := []byte(controllerYAML + `  controller_node: []
  object_store_placement: []
  lease_type:
  - {id: 1, type: application-leadership}
  - {id: 2, type: backup-creation}
  lease:
  - {uuid: leader, lease_type_id: 1, unknown_column: ignored}
  - {uuid: backup, lease_type_id: 2}
  lease_pin:
  - {uuid: leader-pin, lease_uuid: leader}
  - {uuid: backup-pin, lease_uuid: backup}
`)
	for range 2 {
		err := svc.LoadControllerDump(c.Context(), data)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(st.controller, tc.NotNil)
		c.Check(st.controller.Present["controller_node"], tc.IsFalse)
		c.Check(st.controller.Present["object_store_placement"], tc.IsFalse)
		// Replay policy drops source leases and pins without validating
		// their rows, so ownership held by the lost controller never
		// blocks backups on the replacement.
		c.Check(st.controller.Lease, tc.HasLen, 0)
		c.Check(st.controller.LeasePin, tc.HasLen, 0)
		c.Check(st.controller.LeaseType, tc.HasLen, 2)
		c.Check(st.controllerNode, tc.Equals, "bootstrap-node")
	}
}

// Multi-node sources publish address rows for controller nodes that
// recovery never imports. The tables must be skipped, or the archived
// rows would reference missing controller nodes and fail foreign key
// integrity on the replacement.
func (s *serviceSuite) TestControllerAddressTablesSkipped(c *tc.C) {
	st := &recordingState{}
	svc, err := NewService(st, func(context.Context, string) (State, error) { return st, nil }, "bootstrap-node")
	c.Assert(err, tc.ErrorIsNil)
	data := []byte(controllerYAML + `  controller_agent_address:
  - {uuid: agent-0, controller_id: "0", address: 10.0.0.1:17070, scope: local-cloud, priority: 0}
  - {uuid: agent-1, controller_id: "1", address: 10.0.1.1:17070, scope: local-cloud, priority: 0}
  controller_client_address:
  - {uuid: client-0, controller_id: "0", address: 10.0.0.1:17070, scope: public, priority: 0}
  - {uuid: client-2, controller_id: "2", address: 10.0.2.1:17070, scope: public, priority: 0}
  controller_peer_address:
  - {uuid: peer-1, controller_id: "1", address: 10.0.1.1:17070, scope: local-cloud, priority: 0}
  - {uuid: peer-2, controller_id: "2", address: 10.0.2.1:17070, scope: local-cloud, priority: 0}
`)
	err = svc.LoadControllerDump(c.Context(), data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st.controller, tc.NotNil)
	// The replay only touches tables still marked present.
	for _, table := range []string{
		"controller_agent_address",
		"controller_client_address",
		"controller_peer_address",
	} {
		c.Check(st.controller.Present[table], tc.IsFalse, tc.Commentf("table %s", table))
	}
}

func (s *serviceSuite) TestUnknownTableAndColumnRejectedBeforeWrite(c *tc.C) {
	for _, extra := range []string{
		"  unknown_table: []\n",
		"  cloud:\n  - {uuid: cloud, unknown_column: value}\n",
		"  lease_type:\n  - {id: 2, type: backup-creation, unknown_column: value}\n",
	} {
		st := &recordingState{}
		svc, err := NewService(st, func(context.Context, string) (State, error) { return st, nil }, "bootstrap-node")
		c.Assert(err, tc.ErrorIsNil)
		err = svc.LoadControllerDump(c.Context(), []byte(controllerYAML+extra))
		c.Assert(err, tc.NotNil)
		c.Check(st.controller, tc.IsNil)
	}
}

func (s *serviceSuite) TestModelUUIDValidatedBeforeOpening(c *tc.C) {
	opened := false
	svc, err := NewService(&recordingState{}, func(context.Context, string) (State, error) {
		opened = true
		return &recordingState{}, nil
	}, "bootstrap-node")
	c.Assert(err, tc.ErrorIsNil)
	err = svc.LoadModelDump(c.Context(), coremodel.UUID("invalid"), []byte("version: 4.1.0\npayload: {net_node: []}"))
	c.Assert(err, tc.NotNil)
	c.Check(opened, tc.IsFalse)
}

func (s *serviceSuite) TestModelStateReusedAndEmptyTablePreserved(c *tc.C) {
	st := &recordingState{}
	opened := 0
	svc, err := NewService(st, func(context.Context, string) (State, error) { opened++; return st, nil }, "bootstrap-node")
	c.Assert(err, tc.ErrorIsNil)
	model := coremodel.UUID("33333333-0000-0000-0000-000000000000")
	for range 2 {
		err := svc.LoadModelDump(c.Context(), model, []byte("version: 4.1.0\npayload: {net_node: [], k8s_pod: [], k8s_pod_port: []}"))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(st.model, tc.NotNil)
		c.Check(st.model.Present["net_node"], tc.IsTrue)
		c.Check(st.model.Present["machine"], tc.IsFalse)
		// The unit-to-pod mapping is imported so the provisioner can
		// match surviving pods; volatile pod details are not.
		c.Check(st.model.Present["k8s_pod"], tc.IsTrue)
		c.Check(st.model.Present["k8s_pod_port"], tc.IsFalse)
		c.Check(st.modelNode, tc.Equals, "bootstrap-node")
	}
	c.Check(opened, tc.Equals, 1)
}

func (s *serviceSuite) TestFactoryFailureAndCancellation(c *tc.C) {
	failure := errors.New("open failed")
	svc, err := NewService(&recordingState{}, func(context.Context, string) (State, error) { return nil, failure }, "bootstrap-node")
	c.Assert(err, tc.ErrorIsNil)
	model := coremodel.UUID("33333333-0000-0000-0000-000000000000")
	data := []byte("version: 4.1.0\npayload: {net_node: []}")
	err = svc.LoadModelDump(c.Context(), model, data)
	c.Assert(err, tc.ErrorIs, failure)
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	err = svc.LoadModelDump(ctx, model, data)
	c.Assert(err, tc.ErrorIs, context.Canceled)
	err = svc.LoadControllerDump(ctx, []byte(controllerYAML))
	c.Assert(err, tc.ErrorIs, context.Canceled)
}
