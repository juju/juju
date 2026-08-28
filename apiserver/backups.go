// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"net/http"
	"os"

	corebackups "github.com/juju/juju/core/backups"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
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
		http.Error(w, "decoding download args: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Resolve the backup directory from the controller model config. The
	// mirror of juju-backup staging semantics is kept per-request, as 3.6
	// resolved it through the filestorage layer on every Get call.
	domainServices, err := h.domainServicesGetter.ServicesForModel(ctx, h.controllerModelUUID)
	if err != nil {
		http.Error(w, "resolving domain services: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cfg, err := domainServices.Config().ModelConfig(ctx)
	if err != nil {
		http.Error(w, "resolving model config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	backupDir := corebackups.BackupDirToUse(cfg.BackupDir())

	// Reject ids that are not a clearly named archive under the backup dir.
	valid, err := corebackups.IsValidBackupFilepath(backupDir, args.ID)
	if err != nil {
		http.Error(w, "validating archive path: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !valid {
		http.Error(w, "invalid backup archive id", http.StatusBadRequest)
		return
	}

	file, err := os.Open(args.ID)
	if err != nil {
		http.Error(w, "opening archive: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		http.Error(w, "stating archive: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Stream the archive. ServeContent handles range and head requests.
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), file)

	// One-shot semantics: remove the archive after serving it, as 3.6 did.
	if err := os.Remove(args.ID); err != nil && !os.IsNotExist(err) {
		h.logger.Warningf(ctx, "error removing backup archive: %v", err)
	}
}
