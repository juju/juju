// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// AddExecOperation creates an exec operation with tasks for various machines
// and units, using the provided parameters.
func (s *Service) AddExecOperation(
	ctx context.Context,
	target operation.Receivers,
	args operation.ExecArgs,
) (operation.RunResult, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	operationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return operation.RunResult{}, errors.Errorf("generating operation UUID: %w", err)
	}

	// Initialize the results with proper size upfront
	result := operation.RunResult{}
	// We cannot know the exact number of units in advance (because of
	// applications), so we initialize it to the size of the leader targets.
	// This allows us to later consolidate the results.
	result.Units = make([]operation.UnitTaskResult, len(target.LeaderUnit))

	// Initialize a map to store pointers from the leader unit names to the
	// result units, indexed by unit name.
	// This is used to consolidate the results from the state layer.
	leaderUnitsByName := make(map[coreunit.Name]*operation.UnitTaskResult)

	// Initialize leader unit names to pass to the state layer method.
	leaderUnits := make([]coreunit.Name, 0, len(target.LeaderUnit))

	// Process each leader unit target - need to resolve the actual leader units
	for i, appName := range target.LeaderUnit {
		leaderUnit, err := s.leadershipService.ApplicationLeader(appName)
		if err != nil {
			result.Units[i] = operation.UnitTaskResult{
				// Since we haven't gotten the correct leader unit, we need to
				// create an artificial (hard-coded) unit name. This is needed
				// because the API layer consolidates the results based on the
				// receiver name, and it only extracts the application and then
				// concatenates it with "/leader".
				ReceiverName: coreunit.Name(appName + "/0"),
				IsLeader:     true,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("getting leader unit for %s: %w", appName, err),
				},
			}
			continue
		}

		leaderUnitName, err := coreunit.NewName(leaderUnit)
		if err != nil {
			result.Units[i] = operation.UnitTaskResult{
				// Since we haven't gotten the correct leader unit, we need to
				// create an artificial (hard-coded) unit name. This is needed
				// because the API layer consolidates the results based on the
				// receiver name, and it only extracts the application and then
				// concatenates it with "/leader".
				ReceiverName: coreunit.Name(appName + "/0"),
				IsLeader:     true,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("parsing unit name for %s: %w", leaderUnit, err),
				},
			}
			continue
		}

		result.Units[i] = operation.UnitTaskResult{
			ReceiverName: leaderUnitName,
			IsLeader:     true,
		}
		leaderUnitsByName[leaderUnitName] = &result.Units[i]
		leaderUnits = append(leaderUnits, leaderUnitName)
	}

	targetWithResolvedLeaders := internal.ReceiversWithResolvedLeaders{
		Applications: target.Applications,
		Machines:     target.Machines,
		Units:        target.Units,
		LeaderUnits:  leaderUnits,
	}
	// If there are no targets to process, return early.
	if (targetWithResolvedLeaders.Applications == nil ||
		len(targetWithResolvedLeaders.Applications) == 0) &&
		(targetWithResolvedLeaders.Machines == nil ||
			len(targetWithResolvedLeaders.Machines) == 0) &&
		(targetWithResolvedLeaders.Units == nil ||
			len(targetWithResolvedLeaders.Units) == 0) &&
		(targetWithResolvedLeaders.LeaderUnits == nil ||
			len(targetWithResolvedLeaders.LeaderUnits) == 0) {
		return result, nil
	}

	runResult, err := s.st.AddExecOperation(ctx, operationUUID, targetWithResolvedLeaders, args)
	if err != nil {
		return operation.RunResult{}, errors.Errorf("starting exec operation: %w", err)
	}
	result.OperationID = runResult.OperationID
	result.Machines = runResult.Machines

	// Consolidate the leader unit results from the state layer with our
	// pre-computed results.
	for _, unitTaskResult := range runResult.Units {
		if unitTaskResult.IsLeader {
			// If this is a leader unit, we first must match with the
			// pre-computed leader unit result.
			unitResult, ok := leaderUnitsByName[unitTaskResult.ReceiverName]
			if !ok {
				s.logger.Warningf(ctx, "missing results by leader unit for unit %q", unitTaskResult.ReceiverName)
				// This should never happen, but if it does, we'll just skip it.
				continue
			}
			// Update the leader unit result with the actual task info from the
			// state layer.
			unitResult.TaskInfo = unitTaskResult.TaskInfo
			continue
		}
		// This is a regular unit (not a leader), add it directly to the
		// result.
		result.Units = append(result.Units, unitTaskResult)
	}

	// Mark any missing results (leader units that were expected but not
	// returned by the state layer method).
	for unitName, unitResult := range leaderUnitsByName {
		if unitResult.TaskInfo.ID == "" && unitResult.TaskInfo.Error == nil {
			unitResult.TaskInfo.Error = errors.Errorf("missing result for unit %s", unitName)
		}
	}

	return result, nil
}

// AddExecOperationOnAllMachines creates an exec operation with tasks based on
// the provided parameters on all machines.
func (s *Service) AddExecOperationOnAllMachines(
	ctx context.Context,
	args operation.ExecArgs,
) (operation.RunResult, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	operationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return operation.RunResult{}, errors.Errorf("generating operation UUID: %w", err)
	}

	result, err := s.st.AddExecOperationOnAllMachines(ctx, operationUUID, args)
	if err != nil {
		return operation.RunResult{}, errors.Errorf("starting exec operation on all machines: %w", err)
	}

	return result, nil
}

// AddActionOperation creates an action operation with tasks for various units
// using the provided parameters.
func (s *Service) AddActionOperation(
	ctx context.Context,
	target []operation.ActionReceiver,
	args operation.TaskArgs,
) (operation.RunResult, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	for _, t := range target {
		if err := t.Validate(); err != nil {
			return operation.RunResult{}, errors.Errorf("validating action receiver %v: %w", t, err)
		}
	}

	operationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return operation.RunResult{}, errors.Errorf("generating operation UUID: %w", err)
	}

	// Initialize the results with proper size upfront
	result := operation.RunResult{}
	result.Units = make([]operation.UnitTaskResult, len(target))

	// Build slices to track mapping from input receivers to state layer results.
	targetUnits := make([]coreunit.Name, 0, len(target))
	resultSlots := make([]*operation.UnitTaskResult, 0, len(target))

	// Process each target receiver
	for i, unitOrLeader := range target {
		if unitOrLeader.Unit != "" {
			// Direct unit target
			result.Units[i] = operation.UnitTaskResult{
				ReceiverName: unitOrLeader.Unit,
			}
			targetUnits = append(targetUnits, unitOrLeader.Unit)
			resultSlots = append(resultSlots, &result.Units[i])
			continue
		}

		// Leader unit target - need to resolve the actual leader unit
		leaderUnit, err := s.leadershipService.ApplicationLeader(unitOrLeader.LeaderUnit)
		if err != nil {
			result.Units[i] = operation.UnitTaskResult{
				// Since we haven't gotten the correct leader unit, we need to
				// create an artificial (hard-coded) unit name. This is needed
				// because the API layer consolidates the results based on the
				// receiver name, and it only extracts the application and then
				// concatenates it with "/leader".
				ReceiverName: coreunit.Name(unitOrLeader.LeaderUnit + "/0"),
				IsLeader:     true,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("getting leader unit for %s: %w", unitOrLeader.LeaderUnit, err),
				},
			}
			continue
		}

		leaderUnitName, err := coreunit.NewName(leaderUnit)
		if err != nil {
			result.Units[i] = operation.UnitTaskResult{
				// Since we haven't gotten the correct leader unit, we need to
				// create an artificial (hard-coded) unit name. This is needed
				// because the API layer consolidates the results based on the
				// receiver name, and it only extracts the application and then
				// concatenates it with "/leader".
				ReceiverName: coreunit.Name(unitOrLeader.LeaderUnit + "/0"),
				IsLeader:     true,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("parsing unit name for %s: %w", leaderUnit, err),
				},
			}
			continue
		}

		result.Units[i] = operation.UnitTaskResult{
			ReceiverName: leaderUnitName,
			IsLeader:     true,
		}
		targetUnits = append(targetUnits, leaderUnitName)
		resultSlots = append(resultSlots, &result.Units[i])
	}

	// If no valid units to process, return early.
	if len(targetUnits) == 0 {
		return result, nil
	}

	runResult, err := s.st.AddActionOperation(ctx, operationUUID, targetUnits, args)
	if err != nil {
		return operation.RunResult{}, errors.Errorf("adding action operation: %w", err)
	}
	result.OperationID = runResult.OperationID

	// Make sure that we have the same number of result slots as the
	// pre-computed results. This is a sanity check.
	if len(runResult.Units) > len(resultSlots) {
		s.logger.Errorf(ctx, "more state layer results than result slots: %d > %d", len(runResult.Units), len(resultSlots))
		// This should never happen, but if it does, we'll just truncate the
		// state layer results to match the result slots.
		runResult.Units = runResult.Units[:len(resultSlots)]
	}
	// Consolidate the results from the state layer with our pre-computed results.
	for i, unitTaskResult := range runResult.Units {
		resultSlots[i].TaskInfo = unitTaskResult.TaskInfo
	}

	// Mark any missing results (units that were expected but not returned by
	// the state layer method).
	for _, slot := range resultSlots {
		if slot.TaskInfo.ID == "" && slot.TaskInfo.Error == nil {
			slot.TaskInfo.Error = errors.Errorf("missing result for unit %s", slot.ReceiverName)
		}
	}

	return result, nil
}
