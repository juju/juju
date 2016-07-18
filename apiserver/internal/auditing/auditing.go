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

// AuditEntryRecord contains either a valid AuditEntry, or an error.
type AuditEntryRecord struct {
	Value audit.AuditEntry
	Error error
}

// OpenAuditEntriesFn defines a function which will create an
// AuditEntryRecord channel, asynchronously begin placing records on
// it, and return. when called. When the done channel is closed, it is
// expected that the channel which was returned will also be closed.
type OpenAuditEntriesFn func(done <-chan struct{}) <-chan AuditEntryRecord

// Conn defines a client connection.
type Conn interface {

	// Request returns the http request uploaded to the connection.
	Request() *http.Request

	// Send sends data over the connection and handles any marshaling.
	Send(...interface{}) error

	// Close closes the connection.
	Close() error
}

// ConnHandlerContext defines things which NewConnHandler requires to
// operate correctly.
type ConnHandlerContext struct {

	// ServerDone signals when the API server is being torn down. It
	// is expected that when this is received, we should tear
	// everything down.
	ServerDone <-chan struct{}

	// Logger is the logging instance to send messages to.
	Logger loggo.Logger

	// AuthAgent authenticates that the request is coming from a valid
	// agent.
	AuthAgent func(*http.Request) error

	// OpenAuditEntries opens a channel for reading audit entries.
	OpenAuditEntries OpenAuditEntriesFn
}

// Validate ensures the context is valid.
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

// NewConnHandler will return a function which will handle new
// connections of type Conn from a client.
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
		if err := logSendErr(sendError(conn, nil, '\n')); err != nil {
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

func sendError(conn Conn, err error, data ...interface{}) error {
	data = append(
		[]interface{}{
			params.ErrorResult{
				Error: common.ServerError(err),
			},
		},
		data...,
	)
	return conn.Send(data...)
}

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
