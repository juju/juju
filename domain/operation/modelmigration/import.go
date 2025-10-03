// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/objectstore"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/domain/operation/service"
	"github.com/juju/juju/domain/operation/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(
	coordinator Coordinator,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		logger:            logger,
		clock:             clock,
		objectStoreGetter: objectStoreGetter})
}

// ImportService provides a subset of the operation domain service methods
// needed for operation import.
type ImportService interface {
	// ImportOperations sets operations and tasks imported in migration.
	InsertMigratingOperations(ctx context.Context, args internal.ImportOperationsArgs) error

	// DeleteImportedOperations deletes all imported operations in a model during
	// an import rollback.
	DeleteImportedOperations(ctx context.Context) error
}

type importOperation struct {
	modelmigration.BaseOperation

	// injected dependencies.
	objectStoreGetter objectstore.ModelObjectStoreGetter
	clock             clock.Clock
	logger            logger.Logger

	// initialized during Setup.
	service ImportService
}

// Name returns the name of this operation.
func (i *importOperation) Name() string { return "import operations" }

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(
			scope.ModelDB(),
			i.clock,
			i.logger,
		),
		i.clock,
		i.logger,
		i.objectStoreGetter,
		// No leadership service needed for import.
		nil,
	)
	return nil
}

// Execute performs the import of operations and their tasks.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ops := model.Operations()
	if len(ops) == 0 {
		return nil
	}

	args, err := i.buildImportArgs(ctx, model)
	if err != nil {
		return errors.Capture(err)
	}
	if err := i.service.InsertMigratingOperations(ctx, args); err != nil {
		return errors.Errorf("importing operations: %w", err)
	}
	return nil
}

// buildImportArgs builds the arguments for the import operations.
func (i *importOperation) buildImportArgs(ctx context.Context, model description.Model) (internal.ImportOperationsArgs, error) {

	tasksByOp := make(map[string][]internal.ImportTaskArg, len(model.Operations()))
	argsByOp := make(map[string]opTaskArgs, len(model.Operations()))

	for _, task := range model.Actions() {
		machineName, unitName, err := i.resolveMachineOrUnitReceiver(task.Receiver())
		if err != nil {
			i.logger.Warningf(ctx, "cannot find valid receiver for task %q, got %q: %s",
				task.Id(), task.Receiver(), err)
			continue
		}

		if args, ok := argsByOp[task.Operation()]; ok {
			if err := args.check(task, unitName); err != nil {
				return nil, errors.Errorf("inconsistent task args for operation %q: %w", task.Operation(), err)
			}
		} else {
			var application string
			if unitName != "" {
				application = unitName.Application()
			}
			argsByOp[task.Operation()] = opTaskArgs{
				application:    application,
				action:         task.Name(),
				params:         task.Parameters(),
				parallel:       task.Parallel(),
				executionGroup: task.ExecutionGroup(),
			}
		}
		var logs []internal.TaskLogMessage
		if task.Logs() != nil {
			logs = transform.Slice(task.Logs(), func(f description.ActionMessage) internal.TaskLogMessage {
				return internal.TaskLogMessage{
					Timestamp: f.Timestamp(),
					Message:   f.Message(),
				}
			})
		}

		tasksByOp[task.Operation()] = append(tasksByOp[task.Operation()], internal.ImportTaskArg{
			ID:          task.Id(),
			MachineName: machineName,
			UnitName:    unitName,
			Enqueued:    task.Enqueued(),
			Started:     task.Started(),
			Completed:   task.Completed(),
			Status:      corestatus.Status(task.Status()),
			Message:     task.Message(),
			Output:      task.Results(),
			Log:         logs,
			// Note(gfouillet): there is no UUID yet, it will be regenerated
			//   in service layer.
		})
	}

	modelOps := model.Operations()
	result := make(internal.ImportOperationsArgs, 0, len(modelOps))
	for _, op := range modelOps {
		args, _ := argsByOp[op.Id()]
		tasks, _ := tasksByOp[op.Id()]
		delete(tasksByOp, op.Id())

		opArgs := internal.ImportOperationArg{
			ID:             op.Id(),
			Summary:        op.Summary(),
			Enqueued:       op.Enqueued(),
			Started:        op.Started(),
			Completed:      op.Completed(),
			Status:         corestatus.Status(op.Status()),
			Fail:           op.Fail(),
			Tasks:          tasks,
			IsParallel:     args.parallel,
			ExecutionGroup: args.executionGroup,
			Parameters:     args.params,
			Application:    args.application,
			ActionName:     args.action,
			UUID:           "",
		}
		result = append(result, opArgs)
	}

	// check that there is tasks without operation, return an error if any
	if len(tasksByOp) > 0 {
		return nil, errors.Errorf("tasks with unknown operation ids: %v", slices.Collect(maps.Keys(tasksByOp)))
	}

	return result, nil
}

// resolveMachineOrUnitReceiver resolves the receiver name to a machine UUID and a unit name.
func (i *importOperation) resolveMachineOrUnitReceiver(name string) (machine.Name, coreunit.Name,
	error) {
	if name == "" {
		return "", "", errors.Errorf("empty receiver name")
	}

	switch strings.Count(name, "/") {
	case 1: // this is a unit with the name app/0
		return "", coreunit.Name(name), nil
	case 0: // This is a machine with name alike 0
		fallthrough
	case 2: // This is a container machine with name alike 0/lxd/0
		return machine.Name(name), "", nil
	default:
		return "", "", errors.Errorf("invalid receiver name %q", name)
	}
}

// Those params are required to insert value bound to operation,
// but imported with the tasks.
// They should be the same for all tasks.
type opTaskArgs struct {
	application    string // empty for exec operations
	action         string // empty for exec operations
	params         map[string]any
	parallel       bool
	executionGroup string
}

func (o opTaskArgs) check(task description.Action, unit coreunit.Name) error {
	var errs []error
	var application string
	if unit != "" {
		application = unit.Application()
	}
	if o.application != application {
		errs = append(errs, errors.Errorf("application is not consistent across imported tasks got %q and %q",
			o.application, application))
	}
	if o.parallel != task.Parallel() {
		errs = append(errs, errors.Errorf("parallel flag is not consistent across imported tasks"))
	}
	if o.executionGroup != task.ExecutionGroup() {
		errs = append(errs, errors.Errorf("execution group is not consistent across imported tasks got %q and %q",
			o.executionGroup, task.ExecutionGroup()))
	}
	if !reflect.DeepEqual(o.params, task.Parameters()) {
		errs = append(errs, errors.Errorf("parameters are not consistent across imported tasks, got %+v and %+v",
			o.params, task.Parameters()))
	}
	if o.action != task.Name() {
		errs = append(errs, errors.Errorf("action is not consistent across imported tasks, got %q and %q",
			o.action, task.Name()))
	}
	return errors.Join(errs...)
}

// Rollback deletes all imported operations in case of failure.
func (i *importOperation) Rollback(ctx context.Context, model description.Model) error {
	if len(model.Operations()) == 0 {
		return nil
	}
	if err := i.service.DeleteImportedOperations(ctx); err != nil {
		return errors.Errorf("operation import rollback failed: %w", err)
	}
	return nil
}
