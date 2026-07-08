// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/rpc/params"
)

// lokiForwardingNoticeMessage is the notice sent to debug-log clients when
// logs are being forwarded to Loki, informing them that debug-log will not
// receive log lines and the connection will be closed.
const lokiForwardingNoticeMessage = "logs are being forwarded to Loki; debug-log is unavailable"

// sendLokiForwardingNotice sends the initial OK response followed by a
// warning log record informing the client that logs are being forwarded to
// Loki. The caller is expected to close the connection after this returns.
func sendLokiForwardingNotice(socket debugLogSocket, version int) {
	socket.sendOk()
	_ = socket.sendLogRecord(lokiForwardingNoticeRecord(time.Now()), version)
}

// lokiForwardingNoticeRecord returns a warning log record informing the
// client that logs are being forwarded to Loki.
func lokiForwardingNoticeRecord(now time.Time) *params.LogMessage {
	return &params.LogMessage{
		Entity:    "controller",
		Timestamp: now,
		Severity:  corelogger.INFO.String(),
		Module:    "juju.apiserver",
		Message:   lokiForwardingNoticeMessage,
	}
}
