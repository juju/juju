// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/backup"
)

var Backup = backup.Backup
var GetStorage = environs.GetStorage

// backupHandler handles backup requests
type backupHandler struct {
	httpHandler
}

func getMongoConnectionInfo(state *state.State) (info *authentication.MongoInfo) {
	return state.MongoConnectionInfo()
}

var GetMongoConnectionInfo = getMongoConnectionInfo

func (h *backupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.authenticate(r); err != nil {
		h.authError(w, h)
		return
	}

	switch r.Method {
	case "POST":
		file, sha, err := h.doBackup()
		if err != nil {
			h.sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer file.Close()

		if err := uploadBackupToStorage(h.state, file); err != nil {
			h.sendError(w, http.StatusInternalServerError,
				"backup storage failed: "+err.Error())
			return
		}
		// uploadBackupToStorage moved the file position to the end so
		// move it back to the start.
		file.Seek(0, 0)

		w.Header().Set("Content-Type", "application/octet-stream")
		filename := filepath.Base(file.Name())
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.Header().Set("Digest", fmt.Sprintf("SHA=%s", sha))
		w.WriteHeader(http.StatusOK)
		io.Copy(w, file)
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}

// doBackup creates a backup and returns an open file handle to the
// backup archive. The backup file is already deleted when this
// function returns (space will be returned to the OS once the file is
// closed).
func (h *backupHandler) doBackup() (*os.File, string, error) {
	tempDir, err := ioutil.TempDir("", "jujuBackup")
	if err != nil {
		return nil, "", fmt.Errorf("creating backup directory failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	info := GetMongoConnectionInfo(h.state)
	// TODO(dfc) Backup should take a Tag
	var tag string
	if info.Tag != nil {
		tag = info.Tag.String()
	}
	filename, sha, err := Backup(info.Password, tag, tempDir, info.Addrs[0])
	if err != nil {
		return nil, "", fmt.Errorf("backup failed: %v", err)
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, "", fmt.Errorf("backup failed: %v", err)
	}
	return file, sha, nil
}

// sendError sends a JSON-encoded error response.
func (h *backupHandler) sendError(w http.ResponseWriter, statusCode int, message string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	body, err := json.Marshal(&params.BackupResponse{Error: message})
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

// uploadBackupToStorage copies a Juju backup file to environment storage.
var uploadBackupToStorage = func(st *state.State, file *os.File) error {
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat backup file: %v", err)
	}
	stor, err := GetStorage(st)
	if err != nil {
		return fmt.Errorf("failed to open storage: %v", err)
	}
	return stor.Put(backup.StorageName(file.Name()), file, stat.Size())
}
