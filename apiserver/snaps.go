// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"
	"os"
	"path"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/juju/core/paths"
)

// snapDownloadHandler handles snap download through HTTPS in the API server.
type snapDownloadHandler struct {
	ctxt httpContext
}

func newSnapDownloadHandler(httpCtxt httpContext) *snapDownloadHandler {
	return &snapDownloadHandler{
		ctxt: httpCtxt,
	}
}

func (h *snapDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	st, err := h.ctxt.stateForRequestUnauthenticated(r)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	defer st.Release()

	if r.Method != "GET" {
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", r.Method)); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}

	f := r.URL.Query().Get(":object")
	if f == "" {
		if err := sendError(w, errors.BadRequest); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}

	file := path.Base(f)
	if file != f {
		if err := sendError(w, errors.BadRequest); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}

	dir := path.Join(paths.DataDir(paths.CurrentOS()), "snap")
	fullPath := path.Join(dir, file)
	if path.Dir(fullPath) != dir {
		if err := sendError(w, errors.BadRequest); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}

	fileInfo, err := os.Stat(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		if err := sendError(w, errors.NotFound); err != nil {
			logger.Errorf("%v", err)
		}
		return
	} else if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}

	reader, err := os.Open(fullPath)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	w.WriteHeader(200)

	_, _ = io.Copy(w, reader)
}
