// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/fortress"
)

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain-services manifold
	// used to access per-model services for the migrating model.
	DomainServicesName string

	// DomainServicesGetter provides access to domain services for any
	// model by UUID. It is used to resolve controller-model services
	// needed for source prechecks.
	DomainServicesGetter services.DomainServicesGetter

	// FortressName is the name of the fortress manifold that guards
	// model operations during migration.
	FortressName string

	// ModelUUID is the UUID of the model being migrated.
	ModelUUID string

	// LogDir is the path to the controller's log directory. The log
	// transfer collaborator reads logsink.log from this directory.
	LogDir string

	// Clock supplies timing services to the worker.
	Clock clock.Clock

	// NewWorker is called to create a new migrationmaster worker.
	NewWorker func(Config) (worker.Worker, error)
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.DomainServicesGetter == nil {
		return errors.NotValidf("nil DomainServicesGetter")
	}
	if config.FortressName == "" {
		return errors.NotValidf("empty FortressName")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.LogDir == "" {
		return errors.NotValidf("empty LogDir")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.DomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}
	var guard fortress.Guard
	if err := getter.Get(config.FortressName, &guard); err != nil {
		return nil, errors.Trace(err)
	}

	modelUUID := model.UUID(config.ModelUUID)
	controllerModelUUID, err := domainServices.Controller().ControllerModelUUID(context.Background())
	if err != nil {
		return nil, errors.Annotate(err, "resolving controller model UUID")
	}

	sourcePrecheck := func(precheckCtx context.Context) error {
		return migration.SourcePrecheck(
			precheckCtx,
			modelUUID,
			controllerModelUUID,
			domainServices.Model(),
			func(getterCtx context.Context, uuid model.UUID) (migration.ModelMigrationService, error) {
				svc, err := config.DomainServicesGetter.ServicesForModel(getterCtx, uuid)
				if err != nil {
					return nil, errors.Trace(err)
				}
				return svc.ModelMigration(), nil
			},
			func(getterCtx context.Context, uuid model.UUID) (migration.CredentialService, error) {
				svc, err := config.DomainServicesGetter.ServicesForModel(getterCtx, uuid)
				if err != nil {
					return nil, errors.Trace(err)
				}
				return svc.Credential(), nil
			},
			func(getterCtx context.Context, uuid model.UUID) (migration.UpgradeService, error) {
				svc, err := config.DomainServicesGetter.ServicesForModel(getterCtx, uuid)
				if err != nil {
					return nil, errors.Trace(err)
				}
				return svc.Upgrade(), nil
			},
			func(getterCtx context.Context, uuid model.UUID) (migration.ApplicationService, error) {
				svc, err := config.DomainServicesGetter.ServicesForModel(getterCtx, uuid)
				if err != nil {
					return nil, errors.Trace(err)
				}
				return svc.Application(), nil
			},
			func(getterCtx context.Context, uuid model.UUID) (migration.RelationService, error) {
				svc, err := config.DomainServicesGetter.ServicesForModel(getterCtx, uuid)
				if err != nil {
					return nil, errors.Trace(err)
				}
				return svc.Relation(), nil
			},
			func(getterCtx context.Context, uuid model.UUID) (migration.StatusService, error) {
				svc, err := config.DomainServicesGetter.ServicesForModel(getterCtx, uuid)
				if err != nil {
					return nil, errors.Trace(err)
				}
				return svc.Status(), nil
			},
			func(getterCtx context.Context, uuid model.UUID) (migration.ModelAgentService, error) {
				svc, err := config.DomainServicesGetter.ServicesForModel(getterCtx, uuid)
				if err != nil {
					return nil, errors.Trace(err)
				}
				return svc.Agent(), nil
			},
			func(getterCtx context.Context, uuid model.UUID) (migration.MachineService, error) {
				svc, err := config.DomainServicesGetter.ServicesForModel(getterCtx, uuid)
				if err != nil {
					return nil, errors.Trace(err)
				}
				return svc.Machine(), nil
			},
		)
	}

	lgr := internallogger.GetLogger("juju.worker.migrationmaster.manifold", logger.MIGRATION)

	streamModelLog := func(logCtx context.Context, start time.Time) (<-chan common.LogMessage, error) {
		logPath := filepath.Join(config.LogDir, "logsink.log")
		return streamLogsFromFile(logCtx, lgr, logPath, config.ModelUUID, start)
	}

	w, err := config.NewWorker(Config{
		ModelUUID:               config.ModelUUID,
		CharmService:            domainServices.Application(),
		ModelMigrationService:   domainServices.ModelMigration(),
		ExportService:           domainServices.Export(),
		ControllerConfigService: domainServices.ControllerConfig(),
		ModelAgentService:       domainServices.Agent(),
		ResourceService:         domainServices.Resource(),
		Guard:                   guard,
		APIOpen:                 api.Open,
		UploadBinaries:          migration.UploadBinaries,
		AgentBinaryStore:        domainServices.AgentBinaryStore(),
		LoggingService:          domainServices.Logging(),
		Clock:                   config.Clock,
		SourcePrecheck:          sourcePrecheck,
		StreamModelLog:          streamModelLog,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func errorFilter(err error) error {
	switch err := errors.Cause(err); {
	case errors.Is(err, ErrMigrated):
		// If the model has migrated, the migrationmaster should no
		// longer be running.
		return dependency.ErrUninstall
	case errors.Is(err, ErrInactive):
		// If the migration is no longer active, restart the
		// migrationmaster immediately so it can wait for the next
		// attempt.
		return dependency.ErrBounce
	default:
		return err
	}
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
			config.FortressName,
		},
		Start:  config.start,
		Filter: errorFilter,
	}
}

const (
	// logScanBufSize is the initial buffer size for the log scanner.
	logScanBufSize = 64 * 1024

	// logScanMaxTokenSize is the maximum allowed line size for the log
	// scanner. Lines exceeding this size will cause the scanner to stop
	// with bufio.ErrTooLong.
	logScanMaxTokenSize = 1024 * 1024
)

// streamLogsFromFile reads log messages from the local logsink.log file,
// filters by model UUID, and emits them on a channel.
func streamLogsFromFile(ctx context.Context, lgr logger.Logger, logPath, modelUUID string, start time.Time) (<-chan common.LogMessage, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, errors.Annotate(err, "opening log file")
	}

	ch := make(chan common.LogMessage)
	go func() {
		defer close(ch)
		defer func(f *os.File) {
			_ = f.Close()
		}(f)

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, logScanBufSize), logScanMaxTokenSize)
		for scanner.Scan() {
			var record logger.LogRecord
			if err := record.UnmarshalJSON(scanner.Bytes()); err != nil {
				continue
			}
			if record.ModelUUID != modelUUID {
				continue
			}
			if !record.Time.After(start) {
				continue
			}
			select {
			case ch <- common.LogMessage{
				ModelUUID: record.ModelUUID,
				Entity:    record.Entity,
				Timestamp: record.Time,
				Severity:  record.Level.String(),
				Module:    record.Module,
				Location:  record.Location,
				Message:   record.Message,
				Labels:    record.Labels,
			}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			lgr.Errorf(ctx, "error reading log file %s: %v", logPath, err)
		}
	}()
	return ch, nil
}
