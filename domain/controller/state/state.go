// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	domaincontroller "github.com/juju/juju/domain/controller"
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
	db, err := st.DB(ctx)
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
	db, err := st.DB(ctx)
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
	db, err := st.DB(ctx)
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

// GetCACert returns the controller CA certificate.
func (st *State) GetCACert(ctx context.Context) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var cert string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		if cert, err = st.getCACert(ctx, tx); err != nil {
			return err
		}
		return nil
	})

	return cert, errors.Capture(err)
}

// GetControllerInfo returns information about the current controller.
func (st *State) GetControllerInfo(ctx context.Context) (domaincontroller.ControllerInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domaincontroller.ControllerInfo{}, errors.Capture(err)
	}

	var (
		uuid                string
		cert                string
		controllerAddresses []controllerAPIAddress
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		if controllerAddresses, err = st.getAllAPIAddressesForAgents(ctx, tx); err != nil {
			return err
		}
		if uuid, err = st.getControllerUUID(ctx, tx); err != nil {
			return err
		}
		if cert, err = st.getCACert(ctx, tx); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return domaincontroller.ControllerInfo{}, errors.Capture(err)
	}

	return domaincontroller.ControllerInfo{
		UUID:         uuid,
		CACert:       cert,
		APIAddresses: decodeAPIAddresses(controllerAddresses),
	}, nil
}

func (st *State) getControllerUUID(ctx context.Context, tx *sqlair.TX) (string, error) {
	var uuid controllerUUID
	stmt, err := st.Prepare(`
SELECT &controllerUUID.*
FROM   controller
`, uuid)
	if err != nil {
		return "", errors.Errorf("preparing select controller uuid statement: %w", err)
	}
	err = tx.Query(ctx, stmt).Get(&uuid)
	if errors.Is(err, sql.ErrNoRows) {
		// This should never reasonably happen.
		return "", errors.Errorf("internal error: controller uuid not found")
	} else if err != nil {
		return "", errors.Errorf("getting controller uuid: %w", err)
	}

	return uuid.UUID, nil
}

func (st *State) getCACert(ctx context.Context, tx *sqlair.TX) (string, error) {
	stmt, err := st.Prepare("SELECT &caCertValue.* FROM controller", caCertValue{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var cert caCertValue
	err = tx.Query(ctx, stmt).Get(&cert)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.Errorf("no controller CA certificate found")
	} else if err != nil {
		return "", errors.Capture(err)
	}
	return cert.CACert, nil
}

func (st *State) getAllAPIAddressesForAgents(ctx context.Context, tx *sqlair.TX) ([]controllerAPIAddress, error) {
	stmt, err := st.Prepare(`
SELECT &controllerAPIAddress.* 
FROM controller_api_address
WHERE is_agent = true
`, controllerAPIAddress{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []controllerAPIAddress
	err = tx.Query(ctx, stmt).GetAll(&result)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting all api addresses for controller nodes: %w", err)
	}
	return result, nil
}

func decodeAPIAddresses(addrs []controllerAPIAddress) []string {
	var result []string
	for _, addr := range addrs {
		if addr.Address == "" {
			continue
		}

		result = append(result, addr.Address)
	}

	return result
}

// controllerAPIAddress is the database representation of a controller api
// address with the controller id and whether it is for agents or clients.
type controllerAPIAddress struct {
	// Address is the address of the controller node.
	Address string `db:"address"`
}
