// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// auditing provides the API server facade for managing audit
// functionality.
package auditing

import (
	"net/http"

	"golang.org/x/net/websocket"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/audit"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/loggo"
)

type openAuditEntriesFn func(done <-chan struct{}) <-chan audit.AuditEntry

func NewAuditStreamHandler(
	serverDone <-chan struct{},
	logger loggo.Logger,
	authAgent func(*http.Request) error,
	openAuditEntries openAuditEntriesFn,
) http.Handler {
	return websocket.Server{
		Handler: func(conn *websocket.Conn) {
			defer conn.Close()

			connDone := make(chan struct{})
			defer close(connDone)
			done := or(serverDone, connDone)

			if err := authAgent(conn.Request()); err != nil {
				logger.Errorf("%v", errors.Trace(err))
			}

			// Set up a processing pipeline to prepare and
			// serialize records before sending them.
			auditEntryStream := openAuditEntries(done)
			auditEntryParams := recordSerializer(done, auditEntryStream)

			for entry := range auditEntryParams {
				if err := websocket.JSON.Send(conn, entry); err != nil {
					logger.Errorf("%v", errors.Trace(err))
					break
				}
			}
		},
	}
}

func recordSerializer(done <-chan struct{}, entries <-chan audit.AuditEntry) <-chan params.AuditEntryParams {
	entryParamStream := make(chan params.AuditEntryParams)
	go func() {
		defer close(entryParamStream)

		for entry := range entries {
			entryParam, err := auditAPIParamDocFromAuditEntry(entry)
			if err != nil {
				return
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
		Data:              utils.EscapeKeys(auditEntry.Data),
	}, nil
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
