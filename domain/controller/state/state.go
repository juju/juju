// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	controllererrors "github.com/juju/juju/domain/controller/errors"
	"github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetControllerModelUUID returns the model UUID of the controller model.
func (st *State) GetControllerModelUUID(ctx context.Context) (model.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid controllerModelUUID
	stmt, err := st.Prepare(`
SELECT &controllerModelUUID.model_uuid
FROM   controller
`, uuid)
	if err != nil {
		return "", errors.Errorf("preparing select controller model uuid statement: %w", err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			// This should never reasonably happen.
			return errors.Errorf("internal error: controller model uuid not found")
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("getting controller model uuid: %w", err)
	}

	return model.UUID(uuid.UUID), nil
}

// GetControllerAgentInfo returns the information that a controller agent
// needs when it's responsible for serving the API.
func (st *State) GetControllerAgentInfo(ctx context.Context) (controller.ControllerAgentInfo, error) {
	db, err := st.DB()
	if err != nil {
		return controller.ControllerAgentInfo{}, errors.Capture(err)
	}
	var info controllerControllerAgentInfo
	stmt, err := st.Prepare(`SELECT &controllerControllerAgentInfo.* FROM controller`, info)
	if err != nil {
		return controller.ControllerAgentInfo{}, errors.Errorf("preparing select controller agent info statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&info)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("internal error: controller agent info not found").Add(controllererrors.NotFound)
		}
		return err
	})
	if err != nil {
		return controller.ControllerAgentInfo{}, errors.Errorf("getting controller agent info: %w", err)
	}
	return controller.ControllerAgentInfo{
		APIPort:        info.APIPort,
		Cert:           info.Cert,
		PrivateKey:     info.PrivateKey,
		CAPrivateKey:   info.CAPrivateKey,
		SystemIdentity: info.SystemIdentity,
	}, nil
}

// GetModelNamespaces returns the model namespaces of all models in the state.
func (st *State) GetModelNamespaces(ctx context.Context) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`SELECT &namespace.* FROM namespace_list`, namespace{})
	if err != nil {
		return nil, errors.Errorf("preparing select model namespaces statement: %w", err)
	}

	var namespaces []namespace
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&namespaces)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No namespaces found, return an empty slice.
			return nil
		} else if err != nil {
			return errors.Errorf("getting all model namespaces: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting all model namespaces: %w", err)
	}

	return transform.Slice(namespaces, func(ns namespace) string {
		return ns.Namespace
	}), nil
}
