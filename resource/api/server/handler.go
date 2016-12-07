// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"fmt"
	"io"
	"net/http"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource/api"
)

// HTTPHandler is the HTTP handler for the resources endpoint. We use
// it rather having a separate handler for each HTTP method since
// registered API handlers must handle *all* HTTP methods currently.
type HTTPHandler struct {
	// Connect opens a connection to state resources.
	Connect func(*http.Request) (DataStore, names.Tag, error)

	// HandleDownload provides the download functionality.
	HandleDownload func(st DataStore, req *http.Request) (io.ReadCloser, int64, error)

	// HandleUpload provides the upload functionality.
	HandleUpload func(username string, st DataStore, req *http.Request) (*api.UploadResult, error)
}

// NewHTTPHandler creates a new http.Handler for the application
// resources endpoint.
func NewHTTPHandler(connect func(*http.Request) (DataStore, names.Tag, error)) *HTTPHandler {
	return &HTTPHandler{
		Connect: connect,
		HandleDownload: func(st DataStore, req *http.Request) (io.ReadCloser, int64, error) {
			dh := DownloadHandler{
				Store: st,
			}
			return dh.HandleRequest(req)
		},
		HandleUpload: func(username string, st DataStore, req *http.Request) (*api.UploadResult, error) {
			uh := UploadHandler{
				Username: username,
				Store:    st,
			}
			return uh.HandleRequest(req)
		},
	}
}

// ServeHTTP implements http.Handler.
func (h *HTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	st, tag, err := h.Connect(req)
	if err != nil {
		api.SendHTTPError(resp, err)
		return
	}

	switch req.Method {
	case "GET":
		reader, size, err := h.HandleDownload(st, req)
		if err != nil {
			api.SendHTTPError(resp, err)
			return
		}
		defer reader.Close()
		header := resp.Header()
		header.Set("Content-Type", params.ContentTypeRaw)
		header.Set("Content-Length", fmt.Sprint(size))
		resp.WriteHeader(http.StatusOK)
		if _, err := io.Copy(resp, reader); err != nil {
			logger.Errorf("resource download failed: %v", err)
		}
	case "PUT":
		response, err := h.HandleUpload(tagToUsername(tag), st, req)
		if err != nil {
			api.SendHTTPError(resp, err)
			return
		}
		api.SendHTTPStatusAndJSON(resp, http.StatusOK, &response)
	default:
		api.SendHTTPError(resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
	}
}

func tagToUsername(tag names.Tag) string {
	switch tag := tag.(type) {
	case names.UserTag:
		return tag.Name()
	default:
		return ""
	}
}
