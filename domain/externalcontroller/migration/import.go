// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/description/v4"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/externalcontroller"
)

type ImportState interface {
	ImportExternalControllers(context.Context, []externalcontroller.MigrationControllerInfo) error
}

type ImportOperation struct {
	st      ImportState
	stateFn func(database.TxnRunner) (ImportState, error)
}

func (i *ImportOperation) Setup(dbGetter database.DBGetter) error {
	db, err := dbGetter.GetDB(database.ControllerNS)
	if err != nil {
		return errors.Annotatef(err, "retrieving database for import operation")
	}

	i.st, err = i.stateFn(db)
	if err != nil {
		return errors.Annotatef(err, "retrieving state for import operation")
	}

	return nil
}

func (i *ImportOperation) Execute(ctx context.Context, model description.Model) error {
	externalControllers := model.ExternalControllers()
	if len(externalControllers) == 0 {
		return nil
	}

	var docs []externalcontroller.MigrationControllerInfo
	for _, entity := range externalControllers {
		docs = append(docs, externalcontroller.MigrationControllerInfo{
			ControllerTag: entity.ID(),
			Alias:         entity.Alias(),
			CACert:        entity.CACert(),
			Addrs:         entity.Addrs(),
			ModelUUIDs:    entity.Models(),
		})
	}

	err := i.st.ImportExternalControllers(ctx, docs)
	return errors.Annotatef(err, "cannot import external controllers")
}
