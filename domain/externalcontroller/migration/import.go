package migration

import (
	"context"

	"github.com/juju/description/v4"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/database/migration"
	"github.com/juju/juju/domain/externalcontroller"
)

type ImportService interface {
	ImportExternalControllers(context.Context, []externalcontroller.MigrationControllerInfo) error
}

type ImportOperation struct {
	migration.BaseOperation

	service   ImportService
	serviceFn func(database.TxnRunner) (ImportService, error)
}

func (i *ImportOperation) Setup(dbGetter database.DBGetter) error {
	db, err := dbGetter.GetDB(database.ControllerNS)
	if err != nil {
		return errors.Annotatef(err, "cannot get database")
	}

	i.service, err = i.serviceFn(db)
	if err != nil {
		return errors.Annotatef(err, "cannot get service")
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

	err := i.service.ImportExternalControllers(ctx, docs)
	return errors.Annotatef(err, "cannot import external controllers")
}
