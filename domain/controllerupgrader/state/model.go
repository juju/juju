// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/juju/internal/errors"
)

// ControllerModelState provides the means for accessing and modifying the
// controller's model agent version information.
type ControllerModelState struct {
	*domain.StateBase
}

// NewControllerModelState creates a new [ControllerModelState] instance.
func NewControllerModelState(
	factory database.TxnRunnerFactory,
) *ControllerModelState {
	return &ControllerModelState{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetModelTargetAgentVersion returns the target agent version currently set for
// the controller's model. This func expects that the target agent version for
// the model has already been set.
//
// This func will check that the current model is the controller's model and if
// not return an error.
func (s *ControllerModelState) GetModelTargetAgentVersion(
	ctx context.Context,
) (semversion.Number, error) {
	db, err := s.DB()
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	var currentAgentVersion string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		isControllerModel, err := s.isControllerModel(ctx, tx)
		if err != nil {
			return errors.Errorf("checking model is controller model: %w", err)
		}
		if !isControllerModel {
			return errors.New("model being operated on is not the controller's model")
		}

		currentAgentVersion, err = s.getModelTargetAgentVersion(ctx, tx)
		return err
	})

	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	rval, err := semversion.Parse(currentAgentVersion)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"parsing controller model target agent version %q: %w",
			currentAgentVersion, err,
		)
	}
	return rval, nil
}

func (s *ControllerModelState) getModelTargetAgentVersion(
	ctx context.Context,
	tx *sqlair.TX,
) (string, error) {
	var dbVal agentVersionTarget
	stmt, err := s.Prepare(
		"SELECT &agentVersionTarget.* FROM agent_version",
		dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.New("no target agent version has previously been set for the controller's model")
	}

	return dbVal.TargetVersion, err
}

// isControllerModel is a sanity check to ensure that the current model database
// in use is hosting the controller. True is returned when the check passes.
func (s *ControllerModelState) isControllerModel(
	ctx context.Context,
	tx *sqlair.TX,
) (bool, error) {
	var dbVal isControllerModel
	stmt, err := s.Prepare("SELECT &isControllerModel.* FROM model", dbVal)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, errors.Errorf("model information has not been set in the database")
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return dbVal.Is, nil
}

// SetModelTargetAgentVersion is responsible for setting the current target
// agent version of the controller model. This function expects a precondition
// version to be supplied. The model's target agent version at the time the
// operation is applied must match the preCondition version or else an error is
// returned.
//
// This func will check that the current model is the controller's model and if
// not return an error.
func (s *ControllerModelState) SetModelTargetAgentVersion(
	ctx context.Context,
	preCondition semversion.Number,
	toVersion semversion.Number,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	toVersionInput := setAgentVersionTarget{TargetVersion: toVersion.String()}
	setAgentVersionStmt, err := s.Prepare(`
UPDATE agent_version
SET    target_version = $setAgentVersionTarget.target_version
`,
		toVersionInput,
	)
	if err != nil {
		return errors.Capture(err)
	}

	preConditionVersionStr := preCondition.String()
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		isControllerModel, err := s.isControllerModel(ctx, tx)
		if err != nil {
			return errors.Errorf("checking model is controller model: %w", err)
		}
		if !isControllerModel {
			return errors.New("model being operated on is not the controller's model")
		}

		currentAgentVersion, err := s.getModelTargetAgentVersion(ctx, tx)
		if err != nil {
			return errors.Errorf(
				"checking current target agent version for controller model to validate precondition: %w", err,
			)
		}

		if currentAgentVersion != preConditionVersionStr {
			return errors.Errorf(
				"unable to set agent version for controller model. The agent version has changed to %q",
				currentAgentVersion,
			)
		}

		// If the current version is the same as the toVersion we don't need to
		// perform the set operation. This avoids creating any churn in the
		// change log.
		if currentAgentVersion == toVersionInput.TargetVersion {
			return nil
		}

		err = tx.Query(ctx, setAgentVersionStmt, toVersionInput).Run()
		if err != nil {
			return errors.Errorf(
				"setting target agent version to %q for controller model: %w",
				toVersion.String(), err,
			)
		}
		return nil
	})

	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// SetModelTargetAgentVersionAndStream is responsible for setting the
// current target agent version of the controller's model and the agent stream
// that is used. This function expects a precondition version to be supplied.
// The model's target version at the time the operation is applied must match
// the preCondition version or else an error is returned.
//
// This func will check that the current model is the controller's model and if
// not return an error.
func (s *ControllerModelState) SetModelTargetAgentVersionAndStream(
	ctx context.Context,
	preCondition semversion.Number,
	toVersion semversion.Number,
	stream modelagent.AgentStream,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	toVersionStreamInput := setAgentVersionTargetStream{
		StreamID:      int(stream),
		TargetVersion: toVersion.String(),
	}
	setAgentVersionStreamStmt, err := s.Prepare(`
UPDATE agent_version
SET    target_version = $setAgentVersionTargetStream.target_version,
       stream_id = $setAgentVersionTargetStream.stream_id
`,
		toVersionStreamInput,
	)
	if err != nil {
		return errors.Capture(err)
	}

	preConditionVersionStr := preCondition.String()
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		isControllerModel, err := s.isControllerModel(ctx, tx)
		if err != nil {
			return errors.Errorf("checking model is controller model: %w", err)
		}
		if !isControllerModel {
			return errors.New("model being operated on is not the controller's model")
		}

		currentAgentVersion, err := s.getModelTargetAgentVersion(ctx, tx)
		if err != nil {
			return errors.Errorf(
				"checking current target agent version for controller model to validate precondition: %w", err,
			)
		}

		if currentAgentVersion != preConditionVersionStr {
			return errors.Errorf(
				"unable to set agent version and stream for controller model. The agent version has changed to %q",
				currentAgentVersion,
			)
		}

		err = tx.Query(ctx, setAgentVersionStreamStmt, toVersionStreamInput).Run()
		if err != nil {
			return errors.Errorf(
				"setting target agent version and stream for controller model: %w", err,
			)
		}
		return nil
	})

	if err != nil {
		return errors.Capture(err)
	}

	return nil
}
