// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditlog

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/lumberjack/v2"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("core.auditlog")

// Conversation represents a high-level juju command from the juju
// client (or other client). There'll be one Conversation per API
// connection from the client, with zero or more associated
// Request/ResponseErrors pairs.
type Conversation struct {
	Who            string `json:"who"`        // username@idm
	What           string `json:"what"`       // "juju deploy ./foo/bar"
	When           string `json:"when"`       // ISO 8601 to second precision
	ModelName      string `json:"model-name"` // full representation "user/name"
	ModelUUID      string `json:"model-uuid"`
	ConversationID string `json:"conversation-id"` // uint64 in hex
	ConnectionID   string `json:"connection-id"`   // uint64 in hex (using %X to match the value in log files)
}

// ConversationArgs is the information needed to create a method recorder.
type ConversationArgs struct {
	Who          string
	What         string
	ModelName    string
	ModelUUID    string
	ConnectionID uint64
}

// Request represents a call to an API facade made as part of
// a specific conversation.
type Request struct {
	ConversationID string `json:"conversation-id"`
	ConnectionID   string `json:"connection-id"`
	RequestID      uint64 `json:"request-id"`
	When           string `json:"when"`
	Facade         string `json:"facade"`
	Method         string `json:"method"`
	Version        int    `json:"version"`
	Args           string `json:"args,omitempty"`
}

// RequestArgs is the information about an API call that we want to
// record.
type RequestArgs struct {
	Facade    string
	Method    string
	Version   int
	Args      string
	RequestID uint64
}

// ResponseErrors captures any errors coming back from the API in
// response to a request.
type ResponseErrors struct {
	ConversationID string   `json:"conversation-id"`
	ConnectionID   string   `json:"connection-id"`
	RequestID      uint64   `json:"request-id"`
	When           string   `json:"when"`
	Errors         []*Error `json:"errors"`
}

// ResponseErrorsArgs has errors from an API response to record in the
// audit log.
type ResponseErrorsArgs struct {
	RequestID uint64
	Errors    []*Error
}

// Error holds the details of an error sent back from the API.
type Error struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// Record is the top-level entry type in an audit log, which serves as
// a type discriminator. Only one of Conversation/Request/Errors should be set.
type Record struct {
	Conversation *Conversation   `json:"conversation,omitempty"`
	Request      *Request        `json:"request,omitempty"`
	Errors       *ResponseErrors `json:"errors,omitempty"`
}

// AuditLog represents something that can store calls, requests and
// responses somewhere.
type AuditLog interface {
	AddConversation(c Conversation) error
	AddRequest(r Request) error
	AddResponse(r ResponseErrors) error
	Close() error
}

// Recorder records method calls for a specific API connection.
type Recorder struct {
	log          AuditLog
	clock        clock.Clock
	connectionID string
	callID       string
}

// NewRecorder creates a Recorder for the connection described (and
// stores details of the initial call in the log).
func NewRecorder(log AuditLog, clock clock.Clock, c ConversationArgs) (*Recorder, error) {
	callID := newConversationID()
	connectionID := idString(c.ConnectionID)
	err := log.AddConversation(Conversation{
		ConversationID: callID,
		ConnectionID:   connectionID,
		Who:            c.Who,
		What:           c.What,
		When:           clock.Now().Format(time.RFC3339),
		ModelName:      c.ModelName,
		ModelUUID:      c.ModelUUID,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &Recorder{
		log:          log,
		clock:        clock,
		callID:       callID,
		connectionID: connectionID,
	}, nil
}

// AddRequest records a method call to the API.
func (r *Recorder) AddRequest(m RequestArgs) error {
	return errors.Capture(r.log.AddRequest(Request{
		ConversationID: r.callID,
		ConnectionID:   r.connectionID,
		RequestID:      m.RequestID,
		When:           r.clock.Now().Format(time.RFC3339),
		Facade:         m.Facade,
		Method:         m.Method,
		Version:        m.Version,
		Args:           m.Args,
	}))

}

// AddResponse records the result of a method call to the API.
func (r *Recorder) AddResponse(m ResponseErrorsArgs) error {
	return errors.Capture(r.log.AddResponse(ResponseErrors{
		ConversationID: r.callID,
		ConnectionID:   r.connectionID,
		RequestID:      m.RequestID,
		When:           r.clock.Now().Format(time.RFC3339),
		Errors:         m.Errors,
	}))

}

// newConversationID generates a random 64bit integer as hex - this
// will be used to link the requests and responses with the command
// the user issued. We don't use the API server's connection ID here
// because that starts from 0 and increments, so it resets when the
// API server is restarted. The conversation ID needs to be unique
// across restarts, otherwise we'd attribute requests to the wrong
// conversation.
func newConversationID() string {
	buf := make([]byte, 8)
	rand.Read(buf) // Can't fail
	return hex.EncodeToString(buf)
}

type auditLogFile struct {
	fileLogger io.WriteCloser
}

// NewLogFile returns an audit entry sink which writes to an audit.log
// file in the specified directory. maxSize is the maximum size (in
// megabytes) of the log file before it gets rotated. maxBackups is
// the maximum number of old compressed log files to keep (or 0 to
// keep all of them).
func NewLogFile(logDir string, maxSize, maxBackups int) AuditLog {
	logPath := filepath.Join(logDir, "audit.log")
	if err := paths.PrimeLogFile(logPath); err != nil {
		// This isn't a fatal error so log and continue if priming
		// fails.
		logger.Errorf(context.TODO(), "Unable to prime %s (proceeding anyway): %v", logPath, err)
	}

	ljLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		Compress:   true,
	}
	logger.Debugf(context.TODO(), "created rotating log file %q with max size %d MB and max backups %d",
		ljLogger.Filename, ljLogger.MaxSize, ljLogger.MaxBackups)
	return &auditLogFile{
		fileLogger: ljLogger,
	}
}

// AddConversation implements AuditLog.
func (a *auditLogFile) AddConversation(c Conversation) error {
	return errors.Capture(a.addRecord(Record{Conversation: &c}))
}

// AddRequest implements AuditLog.
func (a *auditLogFile) AddRequest(m Request) error {
	return errors.Capture(a.addRecord(Record{Request: &m}))

}

// AddResponse implements AuditLog.
func (a *auditLogFile) AddResponse(m ResponseErrors) error {
	return errors.Capture(a.addRecord(Record{Errors: &m}))
}

// Close implements AuditLog.
func (a *auditLogFile) Close() error {
	return errors.Capture(a.fileLogger.Close())
}

func (a *auditLogFile) addRecord(r Record) error {
	bytes, err := json.Marshal(r)
	if err != nil {
		return errors.Capture(err)
	}
	// Add a linebreak to bytes rather than doing two calls to write
	// just in case lumberjack rolls the file between them.
	bytes = append(bytes, byte('\n'))
	_, err = a.fileLogger.Write(bytes)
	return errors.Capture(err)
}

func idString(id uint64) string {
	return fmt.Sprintf("%X", id)
}
