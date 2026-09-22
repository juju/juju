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
	"strings"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
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
// staged by the Backups facade; the archive is removed once it has
// been fully served. The request body holds the id returned by the
// Create RPC. A corrupted or interrupted transfer can be retried with
// the same id because a partial transfer leaves the archive staged.
// Concurrent requests for the same id are possible: exclusivity is
// not claimed.
type backupsDownloadHandler struct {
	// resolveBackupDir returns the effective backup directory of the
	// controller model.
	resolveBackupDir func(ctx context.Context) (string, error)
	logger           corelogger.Logger
}

// ServeHTTP implements [http.Handler].
func (h *backupsDownloadHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

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
		// Pre-4.1 clients download by archive filename (a bare name or
		// a full path); that client-supplied-path flow is removed. Tell
		// the operator what to do rather than failing with an opaque
		// invalid-id error. This is only string inspection: the id
		// never reaches the filesystem.
		if strings.HasPrefix(args.ID, corebackups.FilenamePrefix) ||
			strings.HasPrefix(filepath.Base(args.ID), corebackups.FilenamePrefix) {
			h.sendError(ctx, w, errors.BadRequestf(
				"downloading backups by filename is not supported"))
			return
		}
		h.sendError(ctx, w, errors.BadRequestf("%v", err))
		return
	}

	h.serveArchive(ctx, w, req, args.ID, archivePath)
}

// serveArchive streams the staged archive at archivePath and removes
// it once the transfer completes. A partial transfer leaves the
// archive staged so the client can retry.
func (h *backupsDownloadHandler) serveArchive(ctx context.Context, w http.ResponseWriter, req *http.Request, id, archivePath string) {
	if req.Header.Get("Range") != "" {
		// The client always fetches the whole archive and verifies it
		// against the recorded checksum; partial reads are not part of
		// that contract, so range requests are rejected rather than
		// served partially.
		h.sendError(ctx, w, errors.BadRequestf("range requests are not supported for backup downloads"))
		return
	}

	file, err := os.Open(archivePath)
	if err != nil {
		if os.IsNotExist(err) {
			// The id is well-formed but nothing is staged for it: the
			// archive was already downloaded.
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

	if _, err := io.Copy(w, file); err != nil {
		// The transfer failed part way: leave the archive staged so
		// the client can retry.
		h.logger.Warningf(ctx, "streaming backup %q to client: %v", id, err)
		return
	}

	// The archive has been fully served, so remove it now: the client
	// keeps a local copy, and if the checksum came back mismatched the
	// corrupt bytes are preserved locally. A failed removal is logged
	// and left for the operator rather than failing an otherwise
	// successful download.
	if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
		h.logger.Warningf(ctx, "removing served backup archive %q: %v", archivePath, err)
	}
}

// sendError logs the error and sends it to the client as a classified
// JSON error. Server-side failures (5xx) are logged at error level:
// the client only ever sees an opaque 500, so the controller log is
// the only place the cause is recorded. Client errors (4xx) stay at
// debug level: they are fully conveyed to the client.
func (h *backupsDownloadHandler) sendError(ctx context.Context, w http.ResponseWriter, err error) {
	if _, status := apiservererrors.ServerErrorAndStatus(err); status >= http.StatusInternalServerError {
		h.logger.Errorf(ctx, "backups download: %v", err)
	} else {
		h.logger.Debugf(ctx, "backups download: %v", err)
	}
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
