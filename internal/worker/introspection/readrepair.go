// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/juju/juju/sockets"
)

const objectStoreReadRepairPath = "/objectstore/read-repair"

type objectStoreReadRepairHandler struct {
	controlSocketPath string
}

func newObjectStoreReadRepairHandler(controlSocketPath string) http.Handler {
	return objectStoreReadRepairHandler{
		controlSocketPath: controlSocketPath,
	}
}

func (h objectStoreReadRepairHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetURL, err := url.Parse("http://unix.socket" + objectStoreReadRepairPath)
	if err != nil {
		http.Error(w, "invalid read-repair endpoint", http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodPost,
		targetURL.String(),
		strings.NewReader("{}"),
	)
	if err != nil {
		http.Error(w, "creating read-repair request failed", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := objectStoreReadRepairClient(h.controlSocketPath).Do(req)
	if err != nil {
		http.Error(w, "requesting read-repair failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		logger.Warningf(r.Context(), "copying read-repair response failed: %v", err)
	}
}

func objectStoreReadRepairClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: objectStoreReadRepairTransport(socketPath),
	}
}

func objectStoreReadRepairTransport(socketPath string) *http.Transport {
	return &http.Transport{
		DialContext: func(
			_ context.Context, _, _ string,
		) (net.Conn, error) {
			return sockets.Dialer(sockets.Socket{
				Network: "unix",
				Address: socketPath,
			})
		},
	}
}
