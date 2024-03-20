// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/juju/version/v2"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/model/state"
	jujuversion "github.com/juju/juju/version"
)

const (
	modelReadonlyDataTimeout = time.Second * 10
)

// CreateModel is responsible for making a new model with all of its associated
// metadata during the bootstrap process.
// If the ModelCreationArgs does not have a credential name set then no cloud
// credential will be associated with the model.
//
// The only supported agent version during bootstrap is that of the current
// controller. This will be the default if no agent version is supplied.
//
// The following error types can be expected to be returned:
// - modelerrors.AlreadyExists: When the model uuid is already in use or a model
// with the same name and owner already exists.
// - errors.NotFound: When the cloud, cloud region, or credential do not exist.
// - errors.NotValid: When the model uuid is not valid.
// - [modelerrors.AgentVersionNotSupported]
func CreateModel(
	args model.ModelCreationArgs,
) (coremodel.UUID, func(context.Context, database.TxnRunner) error) {
	var uuidError error
	uuid := args.UUID
	if uuid == "" {
		uuid, uuidError = coremodel.NewUUID()
	}

	if uuidError != nil {
		return coremodel.UUID(""), func(_ context.Context, _ database.TxnRunner) error {
			return fmt.Errorf("generating bootstrap model %q uuid: %w", args.Name, uuidError)
		}
	}

	return uuid, func(ctx context.Context, db database.TxnRunner) error {
		if err := args.Validate(); err != nil {
			return fmt.Errorf("model creation args: %w", err)
		}

		agentVersion := args.AgentVersion
		if args.AgentVersion == version.Zero {
			agentVersion = jujuversion.Current
		}

		if agentVersion.Compare(jujuversion.Current) != 0 {
			return fmt.Errorf("%w %q during bootstrap", modelerrors.AgentVersionNotSupported, agentVersion)
		}
		args.AgentVersion = agentVersion

		if err := uuid.Validate(); err != nil {
			return fmt.Errorf("invalid model uuid: %w", err)
		}

		return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return state.Create(ctx, uuid, args, tx)
		})
	}
}

// CreateReadOnlyModelInfo
func CreateReadOnlyModelInfo(
	uuid coremodel.UUID,
) (func(context.Context, database.TxnRunner) error, func(context.Context, database.TxnRunner) error) {

	modelInfoCh := make(chan coremodel.Model, 1)
	chClose := sync.OnceFunc(func() {
		close(modelInfoCh)
	})

	controllerConcern := func(ctx context.Context, db database.TxnRunner) error {
		defer chClose()

		if err := uuid.Validate(); err != nil {
			return fmt.Errorf("getting model %q info to create model readonly information: %w", uuid, err)
		}

		var model coremodel.Model
		err := db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			model, err = state.Get(ctx, tx, uuid)
			return err
		})

		if err != nil {
			return fmt.Errorf("getting model %q information for making model database read only information: %w", uuid, err)
		}

		select {
		case _, ok := <-modelInfoCh:
			if !ok {
				return fmt.Errorf("getting model %q information for read only model information. Channel has been closed %w", uuid, err)
			}
		default:
		}

		select {
		case modelInfoCh <- model:
			break
		case <-ctx.Done():
			return fmt.Errorf("waiting to send model %q information for read only model information on channel: %w", uuid, ctx.Err())
		}

		return nil
	}

	modelConcern := func(ctx context.Context, db database.TxnRunner) error {
		ctx, cancel := context.WithTimeout(ctx, modelReadonlyDataTimeout)
		defer cancel()

		var (
			model coremodel.Model
			ok    bool
		)
		select {
		case model, ok = <-modelInfoCh:
			if !ok {
				return fmt.Errorf("reading read only model %q information. Channel is already closed", uuid)
			}
		case <-ctx.Done():
			return fmt.Errorf("reading read only model %q information. %w", uuid, ctx.Err())
		}

		err := db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return state.CreateReadOnlyModel(ctx, tx, model)
		})

		if err != nil {
			return fmt.Errorf("setting model %q read only information: %w", uuid, err)
		}

		return nil
	}

	return controllerConcern, modelConcern
}
