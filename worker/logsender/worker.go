// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"encoding/json"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

// LogRecord represents a log message in an agent which is to be
// transmitted to the JES.
type LogRecord struct {
	Time     time.Time
	Module   string
	Location string
	Level    loggo.Level
	Message  string
}

var logger = loggo.GetLogger("juju.worker.logsender")

// New starts a logsender worker which reads log message structs from
// a channel and sends them to the JES via the logsink API.
func New(logs chan *LogRecord, apiInfo *api.Info) worker.Worker {
	loop := func(stop <-chan struct{}) error {
		logger.Debugf("starting logsender worker")

		conn, err := dialLogsinkAPI(apiInfo)
		if err != nil {
			return errors.Annotate(err, "logsender dial failed")
		}
		defer conn.Close()

		for {
			select {
			case rec := <-logs:
				err := websocket.JSON.Send(conn, &apiserver.LogMessage{
					Time:     rec.Time,
					Module:   rec.Module,
					Location: rec.Location,
					Level:    rec.Level,
					Message:  rec.Message,
				})
				if err != nil {
					// Note: due to the fire-and-forget nature of the
					// logsink API, it is possible that when the
					// connection dies, any logs that were "in-flight"
					// will not be recorded on the server side.
					return errors.Annotate(err, "logsink connection failed")
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
	conn, err := api.Connect(apiInfo, "/logsink", header, api.DefaultDialOpts())
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
		return nil, errors.Annotatef(err, "initial server error")
	}

	return conn, nil
}
