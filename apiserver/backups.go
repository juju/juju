// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"crypto/sha256"
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

// maxDownloadArgsBytes caps the request body read by the download
// handler. The body only ever carries a JSON-encoded archive id, so this
// is far more than a well behaved client needs, and an unbounded body is
// not streamed into the controller.
const maxDownloadArgsBytes = 1 << 20 // 1MiB

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
	r.Body = http.MaxBytesReader(w, r.Body, maxDownloadArgsBytes)
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		h.sendError(ctx, w, jujuerrors.BadRequestf("decoding download args"))
		return
	}

	// Resolve the backup directory from the controller model config. The
	// mirror of juju-backup staging semantics is kept per-request, as 3.6
	// resolved it through the filestorage layer on every Get call.
	domainServices, err := h.domainServicesGetter.ServicesForModel(ctx, h.controllerModelUUID)
	if err != nil {
		h.sendError(ctx, w, errors.Errorf("resolving domain services: %w", err))
		return
	}
	cfg, err := domainServices.Config().ModelConfig(ctx)
	if err != nil {
		h.sendError(ctx, w, errors.Errorf("resolving model config: %w", err))
		return
	}
	backupDir := corebackups.BackupDirToUse(cfg.BackupDir())

	// Reject ids that are not a clearly named archive under the backup dir.
	// Validation resolves symlinks, so a link under the backup dir naming a
	// file elsewhere is accepted: the endpoint is controller-admin only and
	// the backup dir is only writable by the controller itself, so trusting
	// its contents is an accepted trade-off rather than an oversight.
	valid, err := corebackups.IsValidBackupFilepath(backupDir, args.ID)
	if err != nil {
		h.sendError(ctx, w, errors.Errorf("validating archive path: %w", err))
		return
	}
	if !valid {
		h.sendError(ctx, w, jujuerrors.BadRequestf("invalid backup archive id"))
		return
	}

	// An archive that passed validation can still be gone by the time it is
	// opened, because the one-shot removal below races a concurrent or
	// retried download of the same id. That is a bad id, not a controller
	// fault, so it is reported exactly as a validation failure is. Note this
	// must not be NotFound: the client reads NotFound off this endpoint as
	// "this controller does not support backup downloads" and reports
	// success (see cmd/juju/backups/download.go).
	file, err := os.Open(args.ID)
	if errors.Is(err, os.ErrNotExist) {
		h.sendError(ctx, w, jujuerrors.BadRequestf("invalid backup archive id"))
		return
	} else if err != nil {
		h.sendError(ctx, w, errors.Errorf("opening archive: %w", err))
		return
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		h.sendError(ctx, w, errors.Errorf("stating archive: %w", err))
		return
	}

	// ServeContent sets Content-Type from the file extension; the raw
	// archive type and digest headers are set explicitly to match the
	// 3.6 download response.
	w.Header().Set("Content-Type", params.ContentTypeRaw)

	// Compute the archive checksum for the Digest header. The checksum is
	// over the gzipped archive bytes, so a client can verify the download
	// without decompressing it. Note this is a SHA-256 digest, independent
	// of the SHA-1 checksum recorded in the backup metadata.
	//
	// Hashing reads the whole archive, which is only worth doing for a
	// request that will carry the whole archive: a HEAD sends no bytes and
	// a range request sends a slice the digest does not describe, so both
	// are served without it rather than reading the archive twice.
	if r.Method == http.MethodGet && r.Header.Get("Range") == "" {
		checksum, err := archiveChecksum(file)
		if err != nil {
			h.sendError(ctx, w, errors.Errorf("checksumming archive: %w", err))
			return
		}
		w.Header().Set("Digest", params.EncodeChecksum(checksum))
	}

	// Stream the archive. ServeContent handles range and head requests.
	sw := &serveStatusWriter{ResponseWriter: w}
	http.ServeContent(sw, r, fi.Name(), fi.ModTime(), file)

	// One-shot semantics: remove the archive only once it has been
	// served completely. Anything short of a whole-archive 200 response
	// leaves the archive on disk so the download can be retried:
	//   - a HEAD request transfers no bytes at all,
	//   - a range request only sends part of the archive (206),
	//   - a conditional request may send nothing (304),
	//   - a write error means the client went away mid-stream,
	//   - a short body means the copy stopped early.
	if r.Method != http.MethodGet {
		return
	}
	if sw.err != nil {
		h.logger.Warningf(ctx, "error serving backup archive: %v", sw.err)
		return
	}
	if sw.status != http.StatusOK {
		return
	}
	if sw.written != fi.Size() {
		h.logger.Warningf(ctx,
			"backup archive %q served incompletely (%d of %d bytes), keeping it on disk",
			fi.Name(), sw.written, fi.Size())
		return
	}
	if err := os.Remove(args.ID); err != nil && !os.IsNotExist(err) {
		h.logger.Warningf(ctx, "error removing backup archive: %v", err)
	}
}

// serveStatusWriter records the response status, the number of body
// bytes written and the first write error, so the handler can tell a
// completely served archive from a partial or failed response.
//
// It deliberately does not forward [io.ReaderFrom], so the copy runs
// through Write and write errors are observable.
type serveStatusWriter struct {
	http.ResponseWriter
	status  int
	written int64
	err     error
}

func (w *serveStatusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *serveStatusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.written += int64(n)
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}

// sendError logs the internal error detail and replies with a structured
// JSON error, so internal state (paths, model UUIDs, DB errors) is not
// leaked to the client beyond the classified error.
func (h *backupsDownloadHandler) sendError(ctx context.Context, w http.ResponseWriter, err error) {
	h.logger.Debugf(ctx, "backup download error: %v", err)
	if err := internalhttp.SendError(w, err, h.logger); err != nil {
		h.logger.Errorf(ctx, "sending backup download error: %v", err)
	}
}

// archiveChecksum returns the raw SHA-256 checksum of the archive. The
// file offset is reset so the caller can stream the file afterwards.
func archiveChecksum(file *os.File) ([]byte, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, errors.Capture(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Capture(err)
	}
	return hasher.Sum(nil), nil
}
