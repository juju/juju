// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	stderrors "errors"
	"io"
	"net"
	"strings"
	"syscall"

	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/errors"
)

func isTransientLogDeliveryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}

	var netErr net.Error
	if stderrors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	if gorillaws.IsCloseError(err,
		gorillaws.CloseNormalClosure,
		gorillaws.CloseGoingAway,
		gorillaws.CloseNoStatusReceived,
		gorillaws.CloseAbnormalClosure,
	) || gorillaws.IsUnexpectedCloseError(err) {
		return true
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "server returned http status 503") ||
		strings.Contains(message, "service unavailable") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "write failed") ||
		strings.Contains(message, "api caller disconnected") ||
		strings.Contains(message, "api connection is broken") ||
		strings.Contains(message, "connection is shut down") ||
		strings.Contains(message, "connection is closed")
}
