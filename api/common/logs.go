// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// DebugLogParams holds parameters for WatchDebugLog that control the
// filtering of the log messages. If the structure is zero initialized, the
// entire log file is sent back starting from the end, and until the user
// closes the connection.
type DebugLogParams struct {
	// IncludeEntity lists entity tags to include in the response. Tags may
	// include '*' wildcards e.g.: unit-mysql-*, machine-2. If
	// none are set, then all lines are considered included.
	IncludeEntity []string
	// IncludeModule lists logging modules to include in the response. If none
	// are set all modules are considered included.  If a module is specified,
	// all the submodules also match.
	IncludeModule []string
	// IncludeLabel lists logging labels to include in the response. If none
	// are set all labels are considered included.
	IncludeLabels map[string]string
	// ExcludeEntity lists entity tags to exclude from the response. As with
	// IncludeEntity the values may include '*' wildcards.
	ExcludeEntity []string
	// ExcludeModule lists logging modules to exclude from the response. If a
	// module is specified, all the submodules are also excluded.
	ExcludeModule []string
	// ExcludeLabel lists logging labels to exclude from the response.
	ExcludeLabels map[string]string

	// Limit defines the maximum number of lines to return. Once this many
	// have been sent, the socket is closed.  If zero, all filtered lines are
	// sent down the connection until the client closes the connection.
	Limit uint
	// Backlog tells the server to try to go back this many lines before
	// starting filtering. If backlog is zero and replay is false, then there
	// may be an initial delay until the next matching log message is written.
	Backlog uint
	// Level specifies the minimum logging level to be sent back in the response.
	Level loggo.Level
	// Replay tells the server to start at the start of the log file rather
	// than the end. If replay is true, backlog is ignored.
	Replay bool
	// NoTail tells the server to only return the logs it has now, and not
	// to wait for new logs to arrive.
	NoTail bool
	// StartTime should be a time in the past - only records with a
	// log time on or after StartTime will be returned.
	StartTime time.Time
	// Firehose streams logs from all models from the logsink.log file.
	Firehose bool
}

func (args DebugLogParams) URLQuery() url.Values {
	attrs := url.Values{
		"includeEntity": args.IncludeEntity,
		"includeModule": args.IncludeModule,
		"excludeEntity": args.ExcludeEntity,
		"excludeModule": args.ExcludeModule,
	}
	attrs.Set("version", "2")
	var includeLabels []string
	for k, v := range args.IncludeLabels {
		includeLabels = append(includeLabels, fmt.Sprintf("%s=%s", k, v))
	}
	if len(includeLabels) > 0 {
		attrs["includeLabels"] = includeLabels
		// For compatibility with older controllers.
		if loggerTags, ok := args.IncludeLabels[loggo.LoggerTags]; ok {
			attrs["includeLabel"] = strings.Split(loggerTags, ",")
		}
	}
	var excludeLabels []string
	for k, v := range args.ExcludeLabels {
		excludeLabels = append(excludeLabels, fmt.Sprintf("%s=%s", k, v))
	}
	if len(excludeLabels) > 0 {
		attrs["excludeLabels"] = excludeLabels
		// For compatibility with older controllers.
		if loggerTags, ok := args.ExcludeLabels[loggo.LoggerTags]; ok {
			attrs["excludeLabel"] = strings.Split(loggerTags, ",")
		}
	}

	if args.Replay {
		attrs.Set("replay", fmt.Sprint(args.Replay))
	}
	if args.NoTail {
		attrs.Set("noTail", fmt.Sprint(args.NoTail))
	}
	if args.Firehose {
		attrs.Set("firehose", fmt.Sprint(args.Firehose))
	}
	if args.Limit > 0 {
		attrs.Set("maxLines", fmt.Sprint(args.Limit))
	}
	if args.Backlog > 0 {
		attrs.Set("backlog", fmt.Sprint(args.Backlog))
	}
	if args.Level != loggo.UNSPECIFIED {
		attrs.Set("level", fmt.Sprint(args.Level))
	}
	if !args.StartTime.IsZero() {
		attrs.Set("startTime", args.StartTime.Format(time.RFC3339Nano))
	}
	return attrs
}

// LogMessage is a structured logging entry.
type LogMessage struct {
	ModelUUID string
	Entity    string
	Timestamp time.Time
	Severity  string
	Module    string
	Location  string
	Message   string
	Labels    map[string]string
}

// StreamDebugLog requests the specified debug log records from the
// server and returns a channel of the messages that come back.
func StreamDebugLog(ctx context.Context, source base.StreamConnector, args DebugLogParams) (<-chan LogMessage, error) {
	// Prepare URL query attributes.
	attrs := args.URLQuery()

	connection, err := source.ConnectStream(ctx, "/log", attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	messages := make(chan LogMessage)
	go func() {
		defer func() { _ = connection.Close() }()
		defer close(messages)

		for {
			// If the context is done or cancelled, then we can check to ensure
			// that the goroutine is cleaned up.
			if ctx.Err() != nil {
				return
			}

			var msg params.LogMessage
			err := connection.ReadJSON(&msg)
			if err != nil {
				return
			}
			messages <- LogMessage{
				ModelUUID: msg.ModelUUID,
				Entity:    msg.Entity,
				Timestamp: msg.Timestamp,
				Severity:  msg.Severity,
				Module:    msg.Module,
				Location:  msg.Location,
				Message:   msg.Message,
				Labels:    msg.Labels,
			}
		}
	}()

	return messages, nil
}
