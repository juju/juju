// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"

	jujuerrors "github.com/juju/errors"

	internalhttp "github.com/juju/juju/apiserver/internal/http"
	corebackups "github.com/juju/juju/core/backups"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/rpc/params"
)

// backupsDownloadHandler streams a backup archive out of the controller
// model's backup directory over HTTP. The archive is removed from disk on
// successful copy, matching Juju 3.6 one-shot download semantics.
type backupsDownloadHandler struct {
	domainServicesGetter services.DomainServicesGetter
	controllerModelUUID  coremodel.UUID
	logger               corelogger.Logger
}

// ServeHTTP handles a backup download request. The request body carries a
// JSON-encoded [params.BackupsDownloadArgs] naming the archive to fetch.
func (h *backupsDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var args params.BackupsDownloadArgs
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		h.sendError(w, jujuerrors.BadRequestf("decoding download args"))
		return
	}

	// Resolve the backup directory from the controller model config. The
	// mirror of juju-backup staging semantics is kept per-request, as 3.6
	// resolved it through the filestorage layer on every Get call.
	domainServices, err := h.domainServicesGetter.ServicesForModel(ctx, h.controllerModelUUID)
	if err != nil {
		h.sendError(w, errors.Errorf("resolving domain services: %w", err))
		return
	}
	cfg, err := domainServices.Config().ModelConfig(ctx)
	if err != nil {
		h.sendError(w, errors.Errorf("resolving model config: %w", err))
		return
	}
	backupDir := corebackups.BackupDirToUse(cfg.BackupDir())

	// Reject ids that are not a clearly named archive under the backup dir.
	valid, err := corebackups.IsValidBackupFilepath(backupDir, args.ID)
	if err != nil {
		h.sendError(w, errors.Errorf("validating archive path: %w", err))
		return
	}
	if !valid {
		h.sendError(w, jujuerrors.BadRequestf("invalid backup archive id"))
		return
	}

	file, err := os.Open(args.ID)
	if err != nil {
		h.sendError(w, errors.Errorf("opening archive: %w", err))
		return
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		h.sendError(w, errors.Errorf("stating archive: %w", err))
		return
	}

	// Compute the archive checksum for the Digest header, as the 3.6
	// handler did. The checksum is over the gzipped archive bytes.
	checksum, err := archiveChecksum(file)
	if err != nil {
		h.sendError(w, errors.Errorf("checksumming archive: %w", err))
		return
	}

	// ServeContent sets Content-Type from the file extension; the raw
	// archive type and digest headers are set explicitly to match the
	// 3.6 download response.
	w.Header().Set("Content-Type", params.ContentTypeRaw)
	w.Header().Set("Digest", params.EncodeChecksum(checksum))

	// Stream the archive. ServeContent handles range and head requests.
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), file)

	// One-shot semantics: remove the archive after serving it, as 3.6 did.
	if err := os.Remove(args.ID); err != nil && !os.IsNotExist(err) {
		h.logger.Warningf(ctx, "error removing backup archive: %v", err)
	}
}

// sendError logs the internal error detail and replies with a structured
// JSON error, so internal state (paths, model UUIDs, DB errors) is not
// leaked to the client beyond the classified error.
func (h *backupsDownloadHandler) sendError(w http.ResponseWriter, err error) {
	h.logger.Debugf(context.TODO(), "backup download error: %v", err)
	if err := internalhttp.SendError(w, err, h.logger); err != nil {
		h.logger.Errorf(context.TODO(), "sending backup download error: %v", err)
	}
}

// archiveChecksum returns the base64-encoded SHA-1 checksum of the
// archive, matching the format recorded in the backup metadata. The
// file offset is reset so the caller can stream the file afterwards.
func archiveChecksum(file *os.File) (string, error) {
	hasher := sha1.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", errors.Capture(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", errors.Capture(err)
	}
	return base64.StdEncoding.EncodeToString(hasher.Sum(nil)), nil
}
