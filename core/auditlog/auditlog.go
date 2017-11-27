// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package auditlog

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger = loggo.GetLogger("core.auditlog")

// Call represents a high-level juju command from the juju client (or
// other client). There'll be one Call per API connection, with zero
// or more associated FacadeMethods.
type Call struct {
	Who          string `json:"who"`        // username@idm
	What         string `json:"what"`       // "juju deploy ./foo/bar"
	When         string `json:"when"`       // ISO 8601 to second precision
	ModelName    string `json:"model-name"` // full representation "user/name"
	ModelUUID    string `json:"model-uuid"`
	CallID       string `json:"call-id"`       // uint64 in hex
	ConnectionID string `json:"connection-id"` // uint64 in hex (using %X to match the value in log files)
}

// CallArgs is the information needed to create a method recorder.
type CallArgs struct {
	Who          string
	What         string
	When         time.Time
	ModelName    string
	ModelUUID    string
	ConnectionID uint64
}

// FacadeRequest represents a call to an API facade made as part of
// executing a specific high-level command.
type FacadeRequest struct {
	CallID       string `json:"call-id"`
	ConnectionID string `json:"connection-id"`
	RequestID    uint64 `json:"request-id"`
	Facade       string `json:"facade"`
	Method       string `json:"method"`
	Version      int    `json:"version"`
	Args         string `json:"args,omitempty"`
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

// FacadeResponse captures any errors coming back from the API in
// response to a request.
type FacadeResponse struct {
	CallID       string   `json:"call-id"`
	ConnectionID string   `json:"connection-id"`
	RequestID    uint64   `json:"request-id"`
	Errors       []*Error `json:"errors"`
}

// ResponseArgs is the information about an API response to record in
// the audit log.
type ResponseArgs struct {
	RequestID uint64
	Errors    []*Error
}

// Error holds the details of an error sent back from the API.
type Error struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// Record is the top-level entry type in an audit log, which serves as
// a type discriminator. Only one of Call/Request/Response should be set.
type Record struct {
	Call     *Call           `json:"call,omitempty"`
	Request  *FacadeRequest  `json:"request,omitempty"`
	Response *FacadeResponse `json:"response,omitempty"`
}

// AuditLog represents something that can store calls, requests and
// responses somewhere.
type AuditLog interface {
	AddCall(c Call) error
	AddRequest(r FacadeRequest) error
	AddResponse(r FacadeResponse) error
	Close() error
}

// Recorder records method calls for a specific API connection.
type Recorder struct {
	log          AuditLog
	connectionID string
	callID       string
}

// NewRecorder creates a Recorder for the connection described (and
// stores details of the initial call in the log).
func NewRecorder(log AuditLog, c CallArgs) (*Recorder, error) {
	callID := newCallID()
	connectionID := idString(c.ConnectionID)
	err := log.AddCall(Call{
		CallID:       callID,
		ConnectionID: connectionID,
		Who:          c.Who,
		What:         c.What,
		When:         c.When.Format(time.RFC3339),
		ModelName:    c.ModelName,
		ModelUUID:    c.ModelUUID,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Recorder{
		log:          log,
		callID:       callID,
		connectionID: connectionID,
	}, nil
}

// AddRequest records a method call to the API.
func (r *Recorder) AddRequest(m RequestArgs) error {
	return errors.Trace(r.log.AddRequest(FacadeRequest{
		CallID:       r.callID,
		ConnectionID: r.connectionID,
		RequestID:    m.RequestID,
		Facade:       m.Facade,
		Method:       m.Method,
		Version:      m.Version,
		Args:         m.Args,
	}))
}

// AddResponse records the result of a method call to the API.
func (r *Recorder) AddResponse(m ResponseArgs) error {
	return errors.Trace(r.log.AddResponse(FacadeResponse{
		CallID:       r.callID,
		ConnectionID: r.connectionID,
		RequestID:    m.RequestID,
		Errors:       m.Errors,
	}))
}

func newCallID() string {
	buf := make([]byte, 8)
	rand.Read(buf) // Can't fail
	return hex.EncodeToString(buf)
}

type AuditLogFile struct {
	fileLogger io.WriteCloser
}

// NewLogFile returns an audit entry sink which writes to an audit.log
// file in the specified directory.
func NewLogFile(logDir string) *AuditLogFile {
	logPath := filepath.Join(logDir, "audit.log")
	if err := primeLogFile(logPath); err != nil {
		// This isn't a fatal error so log and continue if priming
		// fails.
		logger.Errorf("Unable to prime %s (proceeding anyway): %v", logPath, err)
	}

	return &AuditLogFile{
		fileLogger: &lumberjack.Logger{
			Filename:   logPath,
			MaxSize:    300, // MB
			MaxBackups: 10,
			Compress:   true,
		},
	}
}

// AddCall implements AuditLog.
func (a *AuditLogFile) AddCall(c Call) error {
	return errors.Trace(a.addRecord(Record{Call: &c}))
}

// AddRequest implements AuditLog.
func (a *AuditLogFile) AddRequest(m FacadeRequest) error {
	return errors.Trace(a.addRecord(Record{Request: &m}))

}

// AddResponse implements AuditLog.
func (a *AuditLogFile) AddResponse(m FacadeResponse) error {
	return errors.Trace(a.addRecord(Record{Response: &m}))
}

// Close implements AuditLog.
func (a *AuditLogFile) Close() error {
	return errors.Trace(a.fileLogger.Close())
}

func (a *AuditLogFile) addRecord(r Record) error {
	bytes, err := json.Marshal(r)
	if err != nil {
		return errors.Trace(err)
	}
	// Add a linebreak to bytes rather than doing two calls to write
	// just in case lumberjack rolls the file between them.
	bytes = append(bytes, byte('\n'))
	_, err = a.fileLogger.Write(bytes)
	return errors.Trace(err)
}

func idString(id uint64) string {
	return fmt.Sprintf("%X", id)
}

// primeLogFile ensures the logsink log file is created with the
// correct mode and ownership.
func primeLogFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return errors.Trace(err)
	}
	if err := f.Close(); err != nil {
		return errors.Trace(err)
	}
	err = utils.ChownPath(path, "syslog")
	return errors.Trace(err)
}
