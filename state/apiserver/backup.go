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

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/backup"
)

var Backup = backup.Backup

// backupHandler handles backup requests
type backupHandler struct {
	httpHandler
}

func getMongoConnectionInfo(state *state.State) (info *state.Info) {
	return state.MongoConnectionInfo()
}

var GetMongoConnectionInfo = getMongoConnectionInfo

func (h *backupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.authenticate(r); err != nil {
		h.authError(w, h)
		return
	}

	switch r.Method {
	case "GET":
		file, sha, err := h.doBackup()
		if err != nil {
			h.sendError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Digest", fmt.Sprintf("SHA=%s", sha))

		w.WriteHeader(http.StatusOK)
		io.Copy(w, file)
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}

func (h *backupHandler) doBackup() (*os.File, string, error) {
	tempDir, err := ioutil.TempDir("", "jujuBackup")
	if err != nil {
		return nil, "", fmt.Errorf("creating backup directory failed: %v", err)
	}

	defer os.RemoveAll(tempDir)

	info := GetMongoConnectionInfo(h.state)
	filename, sha, err := Backup(info.Password, info.Tag, tempDir, info.Addrs[0])
	if err != nil {
		return nil, "", fmt.Errorf("backup failed: %v", err)
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, "", fmt.Errorf("backup failed: missing backup file")
	}
	return file, sha, nil
}

// sendError sends a JSON-encoded error response.
func (h *backupHandler) sendError(w http.ResponseWriter, statusCode int, message string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	// We ignore any error here in the interest of at least sending the
	// status code in the response.
	body, _ := json.Marshal(&params.Error{Message: message})
	_, err := w.Write(body)
	if err != nil {
		return err
	}
	return nil
}
