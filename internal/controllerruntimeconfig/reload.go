// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerruntimeconfig

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/juju/errors"
)

// RequestReload notifies controller-local workers that a persisted runtime
// config change is ready to be re-read, by posting to the /reload endpoint
// served over the configchange unix socket at socketPath.
//
// The POST is performed with a 5 second client timeout. The request is
// considered successful when the worker responds with http.StatusNoContent.
func RequestReload(socketPath string) error {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
	req, err := http.NewRequest(http.MethodPost, "http://unix.socket/reload", http.NoBody)
	if err != nil {
		return errors.Annotate(err, "creating controller config reload request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Annotatef(err, "requesting controller config reload via %q", socketPath)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return errors.Errorf("controller config reload via %q failed: %s", socketPath, resp.Status)
	}
	return nil
}
