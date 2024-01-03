// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/rpc/params"
)

// Operations holds a list of operations and whether the
// list has been truncated (used for batch queries).
type Operations struct {
	Operations []Operation
	Truncated  bool
}

// Operation holds a list of actions run as part of the operation
type Operation struct {
	ID        string
	Summary   string
	Fail      string
	Enqueued  time.Time
	Started   time.Time
	Completed time.Time
	Status    string
	Actions   []ActionResult
	Error     error
}

// ActionMessage represents a logged message on an action.
type ActionMessage struct {
	Timestamp time.Time
	Message   string
}

// Action is a named task to execute on a unit or machine.
type Action struct {
	ID         string
	Receiver   string
	Name       string
	Parameters map[string]interface{}
}

// ActionResult is the result of running an action.
type ActionResult struct {
	Action    *Action
	Enqueued  time.Time
	Started   time.Time
	Completed time.Time
	Status    string
	Message   string
	Log       []ActionMessage
	Output    map[string]interface{}
	Error     error
}

// EnqueuedActions represents the result of enqueuing actions to run.
type EnqueuedActions struct {
	OperationID string
	Actions     []ActionResult
}

// ActionSpec is a definition of the parameters and traits of an Action.
// The Params map is expected to conform to JSON-Schema Draft 4 as defined at
// http://json-schema.org/draft-04/schema# (see http://json-schema.org/latest/json-schema-core.html)
type ActionSpec struct {
	Description string
	Params      map[string]interface{}
}

// OperationQueryArgs holds args for listing operations.
type OperationQueryArgs struct {
	Applications []string
	Units        []string
	Machines     []string
	ActionNames  []string
	Status       []string

	// These attributes are used to support client side
	// batching of results.
	Offset *int
	Limit  *int
}

// RunParams is used to provide the parameters to the Run method.
type RunParams struct {
	Commands       string
	Timeout        time.Duration
	Machines       []string
	Applications   []string
	Units          []string
	Parallel       *bool
	ExecutionGroup *string

	// WorkloadContext for CAAS is true when the Commands should be run on
	// the workload not the operator.
	WorkloadContext bool
}

func unmarshallEnqueuedActions(in params.EnqueuedActions) (EnqueuedActions, error) {
	tag, err := names.ParseOperationTag(in.OperationTag)
	if err != nil {
		return EnqueuedActions{}, errors.Trace(err)
	}

	return EnqueuedActions{
		OperationID: tag.Id(),
		Actions:     unmarshallActionResults(in.Actions),
	}, nil
}

func unmarshallActionResults(in []params.ActionResult) []ActionResult {
	result := make([]ActionResult, len(in))
	for i, a := range in {
		result[i] = unmarshallActionResult(a)
	}
	return result
}

func unmarshallActionResult(in params.ActionResult) ActionResult {
	logs := make([]ActionMessage, len(in.Log))
	for i, log := range in.Log {
		logs[i] = ActionMessage{
			Timestamp: log.Timestamp,
			Message:   log.Message,
		}
	}
	var action *Action
	var err error
	if in.Error != nil {
		err = in.Error
	}
	if in.Action != nil {
		tag, tagErr := names.ParseActionTag(in.Action.Tag)
		if tagErr != nil {
			err = tagErr
		} else {
			action = &Action{
				ID:         tag.Id(),
				Receiver:   in.Action.Receiver,
				Name:       in.Action.Name,
				Parameters: in.Action.Parameters,
			}
		}
	}
	return ActionResult{
		Action:    action,
		Enqueued:  in.Enqueued,
		Started:   in.Started,
		Completed: in.Completed,
		Status:    in.Status,
		Message:   in.Message,
		Log:       logs,
		Output:    in.Output,
		Error:     err,
	}
}

func unmarshallOperations(in params.OperationResults) Operations {
	result := Operations{
		Operations: make([]Operation, len(in.Results)),
		Truncated:  in.Truncated,
	}
	for i, op := range in.Results {
		result.Operations[i] = unmarshallOperation(op)
	}
	return result
}

func unmarshallOperation(in params.OperationResult) Operation {
	result := Operation{
		Summary:   in.Summary,
		Fail:      in.Fail,
		Enqueued:  in.Enqueued,
		Started:   in.Started,
		Completed: in.Completed,
		Status:    in.Status,
	}
	if in.Error != nil {
		result.Error = in.Error
		return result
	}
	tag, err := names.ParseOperationTag(in.OperationTag)
	if err != nil {
		return Operation{
			Error: err,
		}
	}
	result.ID = tag.Id()

	result.Actions = make([]ActionResult, len(in.Actions))
	for i, a := range in.Actions {
		result.Actions[i] = unmarshallActionResult(a)
	}
	return result
}

func unmarshallActionSpecs(in map[string]params.ActionSpec) map[string]ActionSpec {
	result := make(map[string]ActionSpec)
	for k, v := range in {
		result[k] = ActionSpec{
			Description: v.Description,
			Params:      v.Params,
		}
	}
	return result
}
