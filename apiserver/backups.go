// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/juju/errors"

	internalhttp "github.com/juju/juju/apiserver/internal/http"
	corebackups "github.com/juju/juju/core/backups"
	corelogger "github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

const (
	// maxBackupsDownloadArgsBytes caps the request body accepted by the
	// backups download endpoint. The body only ever holds a UUID-sized id.
	maxBackupsDownloadArgsBytes = 1 << 10
)

// backupsDownloadHandler streams one-shot downloads of backup archives
// staged by the Backups facade. The request body holds the id returned by
// the Create RPC. Archives are removed from disk once they have been
// fully served, so a given id can be downloaded exactly once (retries of
// an interrupted transfer remain possible within the retention window,
// after which the sweeper removes the archive).
type backupsDownloadHandler struct {
	// resolveBackupDir returns the effective backup directory of the
	// controller model.
	resolveBackupDir func(ctx context.Context) (string, error)
	logger           corelogger.Logger
}

// ServeHTTP implements [http.Handler].
func (h *backupsDownloadHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	if req.Method != http.MethodGet {
		h.sendError(ctx, w, errors.MethodNotAllowedf("unsupported method %q", req.Method))
		return
	}

	req.Body = http.MaxBytesReader(w, req.Body, maxBackupsDownloadArgsBytes)
	defer func() { _ = req.Body.Close() }()
	var args params.BackupsDownloadArgs
	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		h.sendError(ctx, w, errors.BadRequestf("reading request body: %v", err))
		return
	}

	// Backups are a controller-level operation: the backup directory
	// always comes from the controller model's config. The id is only
	// ever resolved to a server-minted filename under the one-shot
	// download directory; it is never treated as a filesystem path.
	backupDir, err := h.resolveBackupDir(ctx)
	if err != nil {
		h.sendError(ctx, w, errors.Trace(err))
		return
	}

	archivePath, err := corebackups.OneShotArchivePath(backupDir, args.ID)
	if err != nil {
		h.sendError(ctx, w, errors.BadRequestf("%v", err))
		return
	}

	h.serveArchive(ctx, w, req, args.ID, archivePath)
}

// serveArchive streams the staged archive at archivePath, removing it on
// a full successful transfer.
func (h *backupsDownloadHandler) serveArchive(ctx context.Context, w http.ResponseWriter, req *http.Request, id, archivePath string) {
	if req.Header.Get("Range") != "" {
		// One-shot semantics: partial reads would defeat the
		// delete-after-download contract, so range requests are
		// rejected rather than served partially.
		h.sendError(ctx, w, errors.BadRequestf("range requests are not supported for backup downloads"))
		return
	}

	file, err := os.Open(archivePath)
	if err != nil {
		if os.IsNotExist(err) {
			// The id is well-formed but nothing is staged for it: the
			// archive was already downloaded or has expired.
			h.sendError(ctx, w, errors.NotFoundf("backup %q", id))
			return
		}
		h.sendError(ctx, w, internalerrors.Errorf("opening backup archive: %w", err))
		return
	}
	defer func() { _ = file.Close() }()

	fi, err := file.Stat()
	if err != nil {
		h.sendError(ctx, w, internalerrors.Errorf("reading backup archive info: %w", err))
		return
	}

	w.Header().Set("Content-Type", params.ContentTypeRaw)
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	w.WriteHeader(http.StatusOK)

	written, err := io.Copy(w, file)
	if err != nil {
		// Partial transfer: leave the archive staged so the client can
		// retry within the retention window.
		h.logger.Warningf(ctx, "streaming backup %q to client: %v", id, err)
		return
	}
	if written < fi.Size() {
		// The client did not take the whole archive; treat it like a
		// partial transfer and keep the archive staged.
		h.logger.Warningf(ctx, "short write streaming backup %q: wrote %d of %d bytes", id, written, fi.Size())
		return
	}

	// Full transfer: the archive has been delivered, so it is removed
	// now. A failed removal is logged and left to the sweeper rather
	// than failing an otherwise successful download.
	if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
		h.logger.Warningf(ctx, "removing downloaded backup archive %q: %v", archivePath, err)
	}
}

// sendError logs the error and sends it to the client as a classified
// JSON error.
func (h *backupsDownloadHandler) sendError(ctx context.Context, w http.ResponseWriter, err error) {
	h.logger.Debugf(ctx, "backups download: %v", err)
	if serr := internalhttp.SendError(w, err, h.logger); serr != nil {
		h.logger.Errorf(ctx, "sending backup download error to client: %v", serr)
	}
}

// resolveBackupDir returns the effective backup directory of the
// controller model.
func (srv *Server) resolveBackupDir(ctx context.Context) (string, error) {
	domainServices, err := srv.shared.domainServicesGetter.ServicesForModel(ctx, srv.shared.controllerModelUUID)
	if err != nil {
		return "", errors.Trace(err)
	}
	modelConfig, err := domainServices.Config().ModelConfig(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	return corebackups.BackupDirToUse(modelConfig.BackupDir()), nil
}
