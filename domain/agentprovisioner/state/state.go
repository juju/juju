package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/environs/config"
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
) (*config.Config, error) {
	if len(keys) == 0 {
		return &config.Config{}, nil
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

	return config.New(false, result)
}

func (s *State) GetControllerConfigKeyValues(
	ctx context.Context,
	keys []string,
) (*controller.Config, error) {
	if len(keys) == 0 {
		return &controller.Config{}, nil
	}

	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	stmt, err := s.Prepare(`
SELECT (key, value) AS &M.*
FROM controller_config
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
			"getting controller config key values: %w",
			domain.CoerceError(err),
		)
	}

	cfg, err := controller.NewConfig("", "", result)
	return &cfg, err
}
