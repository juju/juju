// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// auditing provides the API server facade for managing audit
// functionality.
package auditing

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/audit"
)

type AuditEntryRecord struct {
	Value audit.AuditEntry
	Error error
}

type OpenAuditEntriesFn func(done <-chan struct{}) <-chan AuditEntryRecord

type Conn interface {
	Request() *http.Request
	Send(interface{}) error
	Close() error
}

type ConnHandlerContext struct {
	ServerDone       <-chan struct{}
	Logger           loggo.Logger
	AuthAgent        func(*http.Request) error
	OpenAuditEntries OpenAuditEntriesFn
}

func (c *ConnHandlerContext) Validate() error {
	var nameOfNil string
	switch {
	default:
		return nil
	case c.ServerDone == nil:
		nameOfNil = "ServerDone"
	case c.AuthAgent == nil:
		nameOfNil = "AuthAgent"
	case c.OpenAuditEntries == nil:
		nameOfNil = "OpenAuditEntries"
	}
	return errors.NotAssignedf(nameOfNil)
}

func NewConnHandler(ctx ConnHandlerContext) (func(Conn), error) {
	if err := ctx.Validate(); err != nil {
		return nil, err
	}

	logSendErr := logFailureFn(ctx.Logger.Errorf, "cannot send to client")
	return func(conn Conn) {
		defer conn.Close()

		if err := errors.Trace(ctx.AuthAgent(conn.Request())); err != nil {
			ctx.Logger.Infof(err.Error())
			logSendErr(sendError(conn, err))
			return
		}

		connDone := make(chan struct{})
		defer close(connDone)
		done := or(ctx.ServerDone, connDone)

		// The client is waiting for an indication that the stream
		// is ready (or that it failed).  See
		// api/apiclient.go:readInitialStreamError().
		if err := logSendErr(sendError(conn, nil)); err != nil {
			return
		}

		// Set up a processing pipeline to prepare and
		// serialize records before sending them.
		auditEntryStream := ctx.OpenAuditEntries(done)
		auditEntryParams := recordSerializer(done, auditEntryStream)

		for entry := range auditEntryParams {
			if entry.error != nil {
				logSendErr(sendError(conn, entry.error))
				break
			}
			logSendErr(conn.Send(entry.value))
		}
	}, nil
}

func logFailureFn(log func(string, ...interface{}), message string) func(error) error {
	return func(err error) error {
		if err != nil {
			log("%v", errors.Annotate(err, message))
		}
		return err
	}
}

type serializedAuditEntry struct {
	value params.AuditEntryParams
	error error
}

func recordSerializer(done <-chan struct{}, entries <-chan AuditEntryRecord) <-chan serializedAuditEntry {
	entryParamStream := make(chan serializedAuditEntry)
	go func() {
		defer close(entryParamStream)

		for entry := range entries {
			entryParam := serializedAuditEntry{error: errors.Trace(entry.Error)}
			if entry.Error == nil {
				entryParam.value, entryParam.error = auditAPIParamDocFromAuditEntry(entry.Value)
				entryParam.error = errors.Trace(entryParam.error)
			}

			select {
			case entryParamStream <- entryParam:
			case <-done:
				return
			}
		}
	}()

	return entryParamStream
}

func auditAPIParamDocFromAuditEntry(auditEntry audit.AuditEntry) (params.AuditEntryParams, error) {
	timeAsText, err := auditEntry.Timestamp.MarshalText()
	if err != nil {
		return params.AuditEntryParams{}, errors.Trace(err)
	}

	return params.AuditEntryParams{
		JujuServerVersion: auditEntry.JujuServerVersion,
		ModelUUID:         auditEntry.ModelUUID,
		Timestamp:         string(timeAsText),
		RemoteAddress:     auditEntry.RemoteAddress,
		OriginType:        auditEntry.OriginType,
		OriginName:        auditEntry.OriginName,
		Operation:         auditEntry.Operation,
		Data:              auditEntry.Data,
	}, nil
}

func sendError(conn Conn, err error) error {
	return conn.Send(params.ErrorResult{
		Error: common.ServerError(err),
	})
}

// func sendError(conn *websocket.Conn, err error) error {
// 	initialCodec := websocket.Codec{
// 		Marshal: func(v interface{}) (data []byte, payloadType byte, err error) {
// 			data, payloadType, err = websocket.JSON.Marshal(v)
// 			if err != nil {
// 				return data, payloadType, err
// 			}
// 			// api/apiclient.go:readInitialStreamError() looks for LF.
// 			return append(data, '\n'), payloadType, nil
// 		},
// 	}
// 	return initialCodec.Send(conn, &params.ErrorResult{
// 		Error: common.ServerError(err),
// 	})
// }

func or(c1, c2 <-chan struct{}) <-chan struct{} {
	orChan := make(chan struct{})
	go func() {
		defer close(orChan)
		select {
		case <-c1:
		case <-c2:
		}
	}()
	return orChan
}
