// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	backupsfacade "github.com/juju/juju/apiserver/facades/controller/backups"
	internalhttp "github.com/juju/juju/apiserver/internal/http"
	corebackups "github.com/juju/juju/core/backups"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/rpc/params"
)

const (
	// maxBackupArgsBytes caps the request body accepted by the
	// backups endpoint. The body only ever holds notes or a
	// legacy download id.
	maxBackupArgsBytes = 1 << 10
)

// backupHandler creates backup archives and streams them to the
// client in the same request. The archive is written to a per-request
// temporary directory and removed when the handler returns, however
// the transfer ends: there is no staging, so an interrupted download
// leaves nothing behind and the backup is simply created again.
//
// GET requests are rejected with a clear upgrade error: pre-4.1
// clients downloaded created archives by id, a flow that no longer
// exists.
type backupHandler struct {
	// createArchive builds a fresh backup archive and returns its
	// metadata, its path, and a cleanup that removes it.
	createArchive func(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error)
	logger        corelogger.Logger
}

// ServeHTTP implements [http.Handler].
func (h *backupHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	req.Body = http.MaxBytesReader(w, req.Body, maxBackupArgsBytes)
	defer func() { _ = req.Body.Close() }()

	switch req.Method {
	case http.MethodPost:
		var args params.BackupsCreateArgs
		if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
			h.sendError(ctx, w, errors.BadRequestf("reading request body: %v", err))
			return
		}
		h.createAndServe(ctx, w, args.Notes)
	case http.MethodGet:
		// The legacy download id is not parsed — the flow no longer
		// exists — but drain the body through the MaxBytesReader cap
		// so the limit is enforced and the connection can be reused.
		_, _ = io.Copy(io.Discard, req.Body)
		h.sendError(ctx, w, internalerrors.Errorf(
			"downloading backups from clients older than 4.1 is not supported; use a 4.1 or newer client",
		).Add(coreerrors.NotSupported))
	default:
		h.sendError(ctx, w, errors.MethodNotAllowedf("method %s", req.Method))
	}
}

// createAndServe creates a backup archive, puts its metadata in the
// response headers, and streams the archive as the response body. The
// archive is removed when the handler returns: on a completed
// transfer, on a mid-transfer failure, and on client disconnect.
func (h *backupHandler) createAndServe(ctx context.Context, w http.ResponseWriter, notes string) {
	meta, archivePath, cleanup, err := h.createArchive(ctx, notes)
	if err != nil {
		h.sendError(ctx, w, internalerrors.Capture(err))
		return
	}
	defer cleanup()

	file, err := os.Open(archivePath)
	if err != nil {
		h.sendError(ctx, w, internalerrors.Errorf("opening backup archive: %w", err))
		return
	}
	defer func() { _ = file.Close() }()

	fi, err := file.Stat()
	if err != nil {
		h.sendError(ctx, w, internalerrors.Errorf("reading backup archive info: %w", err))
		return
	}

	result := params.CreateResult(meta, filepath.Base(archivePath))
	metaJSON, err := json.Marshal(result)
	if err != nil {
		h.sendError(ctx, w, internalerrors.Errorf("encoding backup metadata: %w", err))
		return
	}

	w.Header().Set("Content-Type", params.ContentTypeRaw)
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	w.Header().Set(params.BackupMetadataHeader, string(metaJSON))
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, file); err != nil {
		// The transfer failed part way. The archive is removed by the
		// deferred cleanup regardless; the client must create the
		// backup again.
		h.logger.Warningf(ctx, "streaming backup to client: %v", err)
	}
}

// createBackupArchive builds a fresh backup archive using the backups
// facade's creator, resolving the domain services per call, and
// returns its metadata, path, and cleanup.
func (srv *Server) createBackupArchive(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	controllerServices, err := srv.shared.domainServicesGetter.ServicesForModel(ctx, srv.shared.controllerModelUUID)
	if err != nil {
		return nil, "", nil, internalerrors.Capture(err)
	}
	modelServicesFor := backupsfacade.ModelServicesForFunc(
		func(ctx context.Context, modelUUID coremodel.UUID) (backupsfacade.ModelExportDomainServices, error) {
			modelServices, err := srv.shared.domainServicesGetter.ServicesForModel(ctx, modelUUID)
			if err != nil {
				return nil, err
			}
			return modelExportDomainServices{modelServices}, nil
		},
	)
	creator, err := backupsfacade.NewCreator(
		srv.shared.machineTag,
		srv.shared.controllerUUID,
		srv.shared.controllerModelUUID,
		srv.shared.dataDir,
		srv.shared.logDir,
		controllerServices.ControllerExport(),
		modelServicesFor,
		controllerServices.Config(),
		controllerServices.Controller(),
		controllerServices.ControllerNode(),
		srv.shared.clock,
		srv.shared.logger.Child("backups"),
	)
	if err != nil {
		return nil, "", nil, internalerrors.Capture(err)
	}
	return creator.Create(ctx, notes)
}

// modelExportDomainServices adapts a [services.DomainServices] to the
// creator's ModelExportDomainServices interface.
type modelExportDomainServices struct {
	domainServices services.DomainServices
}

// Export returns the model export service.
func (s modelExportDomainServices) Export() backupsfacade.ModelExportService {
	return s.domainServices.Export()
}

// sendError logs the error and sends it to the client as a classified
// JSON error. Server-side failures (5xx) are logged at error level:
// the client only ever sees an opaque 500, so the controller log is
// the only place the cause is recorded. Client errors (4xx) stay at
// debug level: they are fully conveyed to the client.
func (h *backupHandler) sendError(ctx context.Context, w http.ResponseWriter, err error) {
	if _, status := apiservererrors.ServerErrorAndStatus(err); status >= http.StatusInternalServerError {
		h.logger.Errorf(ctx, "backups endpoint: %v", err)
	} else {
		h.logger.Debugf(ctx, "backups endpoint: %v", err)
	}
	if serr := internalhttp.SendError(w, err, h.logger); serr != nil {
		h.logger.Errorf(ctx, "sending backup error to client: %v", serr)
	}
}
