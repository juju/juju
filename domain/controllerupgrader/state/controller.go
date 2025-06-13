// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// ControllerState provides the means for accessing and modifying the
// controllers version information.
type ControllerState struct {
	*domain.StateBase
}

// NewControllerState constructs a new [ControllerState] instance for working
// with the cluster's controller version(s).
func NewControllerState(
	factory database.TxnRunnerFactory,
) *ControllerState {
	return &ControllerState{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetControllerNodeVersions returns the current version that is running for
// each controller in the cluster. This is the version that each controller
// reports when it starts up.
func (s *ControllerState) GetControllerNodeVersions(
	ctx context.Context,
) (map[string]semversion.Number, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT &controllerNodeAgentVersion.*
FROM   controller_node_agent_version
`,
		controllerNodeAgentVersion{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbValues []controllerNodeAgentVersion
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		tx.Query(ctx, stmt).GetAll(&dbValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make(map[string]semversion.Number, len(dbValues))
	for _, v := range dbValues {
		version, err := semversion.Parse(v.Version)
		if err != nil {
			return nil, errors.Errorf(
				"parsing controller node %q agent version %q: %w",
				v.ControllerID, v.Version, err,
			)
		}

		rval[v.ControllerID] = version
	}

	return rval, nil
}

// GetControllerVersion returns the target controller version in use by the
// cluster.
func (s *ControllerState) GetControllerVersion(ctx context.Context) (semversion.Number, error) {
	db, err := s.DB()
	if err != nil {
		return semversion.Number{}, errors.Capture(err)
	}

	var versionValue controllerTargetVersion
	stmt, err := s.Prepare(`
SELECT &controllerTargetVersion.*
FROM   controller
`,
		versionValue)
	if err != nil {
		return semversion.Number{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&versionValue)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("no controller target version has been previously set")
		}
		return err
	})

	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	rval, err := semversion.Parse(versionValue.TargetVersion)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"parsing target version %q for controller: %w",
			versionValue.TargetVersion, err,
		)
	}

	return rval, nil
}

// SetControllerTargetVersion is responsible for setting the current clusters
// target controller version. Controllers in the cluster will eventually
// upgrade to this version once changed.
//
// This func expects that a controller version has already been set. If this is
// not the case no update will be performed and an error will be returned. It is
// not the responsibility of this function to establish the initial controller
// information.
func (s *ControllerState) SetControllerTargetVersion(
	ctx context.Context,
	version semversion.Number,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	toVersionInput := setControllerTargetVersion{
		TargetVersion: version.String(),
	}
	stmt, err := s.Prepare(`
UPDATE controller
SET    target_version = $setControllerTargetVersion.target_version
`,
		toVersionInput)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, toVersionInput).Get(&outcome)
	})
	if err != nil {
		return errors.Capture(err)
	}

	updateCount, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Errorf("getting update count after setting controller version: %w", err)
	}
	if updateCount == 0 {
		return errors.New("no controller version has been previously set")
	}

	return nil
}
