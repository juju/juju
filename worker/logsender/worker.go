// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/gate"
)

const loggerName = "juju.worker.logsender"

var logger = loggo.GetLogger(loggerName)

// New starts a logsender worker which reads log message structs from
// a channel and sends them to the JES via the logsink API.
func New(logs LogRecordCh, apiInfoGate gate.Waiter, agent agent.Agent) worker.Worker {
	loop := func(stop <-chan struct{}) error {
		logger.Debugf("started log-sender worker; waiting for api info")
		select {
		case <-apiInfoGate.Unlocked():
		case <-stop:
			return nil
		}

		logger.Debugf("dialing log-sender connection")
		apiInfo := agent.CurrentConfig().APIInfo()
		conn, err := dialLogsinkAPI(apiInfo)
		if err != nil {
			return errors.Annotate(err, "logsender dial failed")
		}
		defer conn.Close()

		for {
			select {
			case rec := <-logs:
				err := sendLogRecord(conn, rec.Time, rec.Module, rec.Location, rec.Level, rec.Message)
				if err != nil {
					return errors.Trace(err)
				}
				if rec.DroppedAfter > 0 {
					// If messages were dropped after this one, report
					// the count (the source of the log messages -
					// BufferedLogWriter - handles the actual dropping
					// and counting).
					//
					// Any logs indicated as dropped here are will
					// never end up in the logs DB in the JES
					// (although will still be in the local agent log
					// file). Message dropping by the
					// BufferedLogWriter is last resort protection
					// against memory exhaustion and should only
					// happen if API connectivity is lost for extended
					// periods. The maximum in-memory log buffer is
					// quite large (see the InstallBufferedLogWriter
					// call in jujuDMain).
					err := sendLogRecord(conn, rec.Time, loggerName, "", loggo.WARNING,
						fmt.Sprintf("%d log messages dropped due to lack of API connectivity", rec.DroppedAfter))
					if err != nil {
						return errors.Trace(err)
					}
				}

			case <-stop:
				return nil
			}
		}
	}
	return worker.NewSimpleWorker(loop)
}

func dialLogsinkAPI(apiInfo *api.Info) (*websocket.Conn, error) {
	// TODO(mjs) Most of this should be extracted to be shared for
	// connections to both /log (debuglog) and /logsink.
	header := utils.BasicAuthHeader(apiInfo.Tag.String(), apiInfo.Password)
	header.Set("X-Juju-Nonce", apiInfo.Nonce)
	conn, err := api.Connect(apiInfo, "/logsink", header, api.DialOpts{})
	if err != nil {
		return nil, errors.Annotate(err, "failed to connect to logsink API")
	}

	// Read the initial error and translate to a real error.
	// Read up to the first new line character. We can't use bufio here as it
	// reads too much from the reader.
	line := make([]byte, 4096)
	n, err := conn.Read(line)
	if err != nil {
		return nil, errors.Annotate(err, "unable to read initial response")
	}
	line = line[0:n]

	var errResult params.ErrorResult
	err = json.Unmarshal(line, &errResult)
	if err != nil {
		return nil, errors.Annotate(err, "unable to unmarshal initial response")
	}
	if errResult.Error != nil {
		return nil, errors.Annotatef(errResult.Error, "initial server error")
	}

	return conn, nil
}

func sendLogRecord(conn *websocket.Conn, ts time.Time, module, location string, level loggo.Level, msg string) error {
	err := websocket.JSON.Send(conn, &apiserver.LogMessage{
		Time:     ts,
		Module:   module,
		Location: location,
		Level:    level,
		Message:  msg,
	})
	// Note: due to the fire-and-forget nature of the
	// logsink API, it is possible that when the
	// connection dies, any logs that were "in-flight"
	// will not be recorded on the server side.
	return errors.Annotate(err, "logsink connection failed")
}
