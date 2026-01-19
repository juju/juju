// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	applicationmodelmigration "github.com/juju/juju/domain/application/modelmigration"
	machinemodelmigration "github.com/juju/juju/domain/machine/modelmigration"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	"github.com/juju/juju/domain/operation"
	operationmodelmigration "github.com/juju/juju/domain/operation/modelmigration"
	"github.com/juju/juju/domain/operation/service"
	"github.com/juju/juju/domain/operation/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	schematesting.ModelSuite
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) createApplication(desc description.Model, appName string) description.Application {
	app := desc.AddApplication(description.ApplicationArgs{
		Name:     appName,
		CharmURL: "ch:" + appName + "-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/22.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: appName,
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})
	return app
}

func (s *importSuite) TestImportApplicationOperation(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})
	desc.AddMachine(description.MachineArgs{
		Id:   "0",
		Base: "ubuntu@22.04",
	})
	// Add an application to link the operation too
	appFoo := s.createApplication(desc, "foo")
	appFoo.AddUnit(description.UnitArgs{
		Name:    "foo/0",
		Machine: "0",
	})
	appFoo.SetCharmActions(description.CharmActionsArgs{Actions: map[string]description.CharmAction{
		"do-it": migrationtesting.Action{
			Description_: "do-it description",
		},
	}})

	now := time.Now().UTC().Truncate(time.Second)
	desc.AddOperation(description.OperationArgs{
		Id:        "op-1",
		Summary:   "test op",
		Enqueued:  now.Add(-3 * time.Hour),
		Started:   now.Add(-2 * time.Hour),
		Completed: now.Add(-1 * time.Hour),
		Status:    corestatus.Completed.String(),
	})
	desc.AddAction(description.ActionArgs{
		Id:        "a-1",
		Receiver:  "foo/0",
		Name:      "do-it",
		Operation: "op-1",
		Parameters: map[string]any{
			"p1": "v1",
		},
		Parallel:       true,
		ExecutionGroup: "grp-1",
		Enqueued:       now.Add(-3 * time.Hour),
		Started:        now.Add(-2 * time.Hour),
		Completed:      now.Add(-1 * time.Hour),
		Status:         corestatus.Completed.String(),
		Message:        "action completed",
		Messages: []description.ActionMessage{
			migrationtesting.ActionMessage{Timestamp_: now.Add(-90 * time.Minute),
				Message_: "action completed successfully"},
		},
	})

	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	machinemodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	applicationmodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	operationmodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	err := coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, "deadbeef", model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)

	svc := s.setupService(c)
	ops, err := svc.GetOperations(c.Context(), operation.QueryArgs{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(ops.Operations, tc.HasLen, 1)
	op := ops.Operations[0]
	c.Check(op.Summary, tc.Equals, "test op")
	c.Check(op.Status, tc.Equals, corestatus.Completed)
	c.Check(op.Enqueued.Equal(now.Add(-3*time.Hour)), tc.IsTrue)
	c.Check(op.Started.Equal(now.Add(-2*time.Hour)), tc.IsTrue)
	c.Check(op.Completed.Equal(now.Add(-1*time.Hour)), tc.IsTrue)

	c.Assert(op.Units, tc.HasLen, 1)
	task := op.Units[0]
	c.Check(task.ID, tc.Equals, "a-1")
	c.Check(task.ReceiverName, tc.Equals, coreunit.Name("foo/0"))
	c.Check(task.ActionName, tc.Equals, "do-it")
	c.Check(task.Status, tc.Equals, corestatus.Completed)
	c.Assert(task.ExecutionGroup, tc.NotNil)
	c.Check(*task.ExecutionGroup, tc.Equals, "grp-1")
	c.Check(task.IsParallel, tc.IsTrue)
	c.Check(task.Parameters, tc.DeepEquals, map[string]any{"p1": "v1"})
	c.Check(task.Log, tc.HasLen, 1)
	c.Check(task.Log[0].Timestamp.Equal(now.Add(-90*time.Minute)), tc.IsTrue)
	c.Check(task.Log[0].Message, tc.Equals, "action completed successfully")
	c.Check(task.Message, tc.Equals, "action completed")
}

func (s *importSuite) TestImportMachineOperation(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	desc.AddMachine(description.MachineArgs{
		Id:   "0",
		Base: "ubuntu@22.04",
	})

	now := time.Now().UTC().Truncate(time.Second)
	desc.AddOperation(description.OperationArgs{
		Id:       "op-2",
		Summary:  "machine op",
		Enqueued: now.Add(-3 * time.Hour),
		Started:  now.Add(-2 * time.Hour),
		Status:   corestatus.Running.String(),
	})
	desc.AddAction(description.ActionArgs{
		Id:        "a-2",
		Receiver:  "0",
		Name:      "juju-exec",
		Operation: "op-2",
		Enqueued:  now.Add(-3 * time.Hour),
		Started:   now.Add(-2 * time.Hour),
		Status:    corestatus.Running.String(),
		Parameters: map[string]any{
			"command": "ls",
		},
	})

	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	machinemodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	operationmodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	err := coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, "deadbeef", model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)

	svc := s.setupService(c)
	ops, err := svc.GetOperations(c.Context(), operation.QueryArgs{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(ops.Operations, tc.HasLen, 1)
	op := ops.Operations[0]
	c.Check(op.Summary, tc.Equals, "machine op")
	c.Check(op.Status, tc.Equals, corestatus.Running)

	c.Assert(op.Machines, tc.HasLen, 1)
	task := op.Machines[0]
	c.Check(task.ID, tc.Equals, "a-2")
	c.Check(task.ReceiverName, tc.Equals, machine.Name("0"))
	c.Check(task.ActionName, tc.Equals, "juju-exec")
	c.Check(task.Parameters, tc.DeepEquals, map[string]any{"command": "ls"})
}

func (s *importSuite) setupService(c *tc.C) *service.Service {
	modelDB := func(context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	st := state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))
	return service.NewService(
		st,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
		nil,
		&fakeLeadershipService{},
	)
}

type fakeLeadershipService struct{}

func (f *fakeLeadershipService) ApplicationLeader(appName string) (string, error) {
	return "", fmt.Errorf("not found")
}
