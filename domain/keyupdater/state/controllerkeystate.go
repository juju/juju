// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

// ControllerKeyState provides a state access layer for accessing a controller's
// ssh keys via controller config.
type ControllerKeyState struct {
	*domain.StateBase
}

// GetControllerConfigKeys returns the controller config key and values for the
// keys supplied. If one or more keys supplied do not exist in the controller's
// config they will be omitted from the final result.
func (st *ControllerKeyState) GetControllerConfigKeys(
	ctx context.Context,
	keys []string,
) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	sqlKeys := make(sqlair.S, 0, len(keys))
	for _, key := range keys {
		sqlKeys = append(sqlKeys, key)
	}

	stmt, err := st.Prepare(`
SELECT &keyValue.*
FROM v_controller_config
WHERE key IN ($S[:])
`, keyValue{}, sqlKeys)
	if err != nil {
		return nil, errors.Trace(err)
	}

	keyValues := []keyValue{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sqlKeys).GetAll(&keyValues)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf(
			"cannot get controller config for keys %v: %w",
			keys, err,
		)
	}

	rval := make(map[string]string, len(keyValues))
	for _, kv := range keyValues {
		rval[kv.Key] = kv.Value
	}

	return rval, nil
}

// NewControllerKeyState constructs a new state for interacting with the
// underlying authorised keys of a controller via controller config.
func NewControllerKeyState(factory database.TxnRunnerFactory) *ControllerKeyState {
	return &ControllerKeyState{
		StateBase: domain.NewStateBase(factory),
	}
}
