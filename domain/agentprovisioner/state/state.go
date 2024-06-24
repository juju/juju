package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

type State struct {
	*domain.StateBase
}

func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

func (s *State) GetModelConfigKeyValues(
	ctx context.Context,
	keys []string,
) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}

	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	stmt, err := s.Prepare(`
SELECT (key, value) AS &M.*
FROM model_config
WHERE key in ($S[:])
`, sqlair.S{}, sqlair.M{})

	if err != nil {
		return nil, fmt.Errorf(
			"preparing get model config key values: %w", domain.CoerceError(err),
		)
	}

	input := make(sqlair.S, 0, len(keys))
	for _, key := range keys {
		input = append(input, key)
	}
	result := make(sqlair.M, len(keys))

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, &input).Get(result)
	})

	if err != nil {
		return nil, fmt.Errorf(
			"getting model config key values: %w",
			domain.CoerceError(err),
		)
	}

	rval := make(map[string]string, len(result))
	for k, v := range result {
		rval[k] = v.(string)
	}

	return rval, nil
}
