// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	coreactions "github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/watcher"
)

var logger = loggo.GetLogger("juju.cmd.juju.action")

// addValueToMap adds the given value to the map on which the method is run.
// This allows us to merge maps such as {foo: {bar: baz}} and {foo: {baz: faz}}
// into {foo: {bar: baz, baz: faz}}.
func addValueToMap(keys []string, value interface{}, target map[string]interface{}) {
	next := target

	for i := range keys {
		// If we are on last key set or overwrite the val.
		if i == len(keys)-1 {
			next[keys[i]] = value
			break
		}

		if iface, ok := next[keys[i]]; ok {
			switch typed := iface.(type) {
			case map[string]interface{}:
				// If we already had a map inside, keep
				// stepping through.
				next = typed
			default:
				// If we didn't, then overwrite value
				// with a map and iterate with that.
				m := map[string]interface{}{}
				next[keys[i]] = m
				next = m
			}
			continue
		}

		// Otherwise, it wasn't present, so make it and step
		// into.
		m := map[string]interface{}{}
		next[keys[i]] = m
		next = m
	}
}

const (
	watchTimestampFormat  = "15:04:05"
	resultTimestampFormat = "2006-01-02T15:04:05"
)

func decodeLogMessage(encodedMessage string, utc bool) (string, error) {
	var actionMessage coreactions.ActionMessage
	err := json.Unmarshal([]byte(encodedMessage), &actionMessage)
	if err != nil {
		return "", errors.Trace(err)
	}
	return formatLogMessage(actionMessage, true, utc, true), nil
}

func formatTimestamp(timestamp time.Time, progressFormat, utc, plain bool) string {
	if timestamp.IsZero() {
		return ""
	}
	if utc {
		timestamp = timestamp.UTC()
	} else {
		timestamp = timestamp.Local()
	}
	if !progressFormat && !plain {
		return timestamp.String()
	}
	timestampFormat := resultTimestampFormat
	if progressFormat {
		timestampFormat = watchTimestampFormat
	}
	return timestamp.Format(timestampFormat)
}

func formatLogMessage(actionMessage coreactions.ActionMessage, progressFormat, utc, plain bool) string {
	return fmt.Sprintf("%v %v", formatTimestamp(actionMessage.Timestamp, progressFormat, utc, plain), actionMessage.Message)
}

// processLogMessages starts a go routine to decode and handle any incoming
// action log messages received via the string watcher.
func processLogMessages(
	w watcher.StringsWatcher, done chan struct{}, ctx *cmd.Context, utc bool, handler func(*cmd.Context, string),
) {
	go func() {
		defer w.Kill()
		for {
			select {
			case <-done:
				return
			case messages, ok := <-w.Changes():
				if !ok {
					return
				}
				for _, msg := range messages {
					logMsg, err := decodeLogMessage(msg, utc)
					if err != nil {
						logger.Warningf("badly formatted action log message: %v\n%v", err, msg)
						continue
					}
					handler(ctx, logMsg)
				}
			}
		}
	}()
}
