// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package auditlog

import (
	"encoding/hex"
	"io"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v2"
)

// Call represents a high-level juju command from the juju client (or
// other client). There'll be one Call per API connection, with zero
// or more associated FacadeMethods.
type Call struct {
	Who          string `yaml:"who"`        // username@idm
	What         string `yaml:"what"`       // "juju deploy ./foo/bar"
	When         string `yaml:"when"`       // ISO 8601 to second precision
	ModelName    string `yaml:"model-name"` // full representation "user/name"
	ModelUUID    string `yaml:"model-uuid"`
	ConnectionID string `yaml:"connection-id"` // uint64 in hex
}

// CallArgs is the information needed to create a method recorder.
type CallArgs struct {
	Who       string
	What      string
	When      string
	ModelName string
	ModelUUID string
}

// FacadeMethod represents a call to an API facade made as part of
// executing a specific high-level command.
type FacadeMethod struct {
	ConnectionID string `yaml:"connection-id"`
	Facade       string `yaml:"facade"`
	Method       string `yaml:"method"`
	Version      int    `yaml:"version"`
	Args         string `yaml:"args"`
}

// MethodArgs is the information about an API call that we want to
// record.
type MethodArgs struct {
	Facade  string
	Method  string
	Version int
	Args    string
}

type Record struct {
	Call   Call         `yaml:"call"`
	Method FacadeMethod `yaml:"method"`
}

// AuditLog represents something that can store calls and methods
// somewhere.
type AuditLog interface {
	AddCall(c Call) error
	AddMethod(m FacadeMethod) error
}

// Recorder records method calls for a specific API connection.
type Recorder struct {
	log          AuditLog
	connectionID string
}

// NewRecorder creates a Recorder for the connection described (and
// stores details of the initial call in the log).
func NewRecorder(log AuditLog, c CallArgs) (*Recorder, error) {
	connectionID := newConnectionID()
	err := log.AddCall(Call{
		ConnectionID: connectionID,
		Who:          c.Who,
		What:         c.What,
		When:         c.When,
		ModelName:    c.ModelName,
		ModelUUID:    c.ModelUUID,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Recorder{log: log, connectionID: connectionID}, nil
}

// Add records a method call to the API.
func (r *Recorder) Add(m MethodArgs) error {
	return errors.Trace(r.log.AddMethod(FacadeMethod{
		ConnectionID: r.connectionID,
		Facade:       m.Facade,
		Method:       m.Method,
		Version:      m.Version,
		Args:         m.Args,
	}))
}

func newConnectionID() string {
	buf := make([]byte, 8)
	rand.Read(buf) // Can't fail
	return hex.EncodeToString(buf)
}

type AuditLogFile struct {
	fileLogger io.WriteCloser
}

// NewLogFileSink returns an audit entry sink which writes
// to an audit.log file in the specified directory.
func NewLogFile(logDir string) *AuditLogFile {
	logPath := filepath.Join(logDir, "audit-log.yaml")
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

func (a *AuditLogFile) AddCall(c Call) error {
	return errors.Trace(a.addRecord(Record{Call: c}))
}

func (a *AuditLogFile) AddMethod(m FacadeMethod) error {
	return errors.Trace(a.addRecord(Record{Method: m}))
}

func (a *AuditLogFile) Close() error {
	return errors.Trace(a.fileLogger.Close())
}

const documentStart = "---\n"

func (a *AuditLogFile) addRecord(r Record) error {
	bytes, err := yaml.Marshal(r)
	if err != nil {
		return errors.Trace(err)
	}
	// Combining the start and document together in one write to
	// prevent lumberjack from rolling the file between them.
	withStart := make([]byte, 0, len(documentStart)+len(bytes))
	withStart = append(withStart, []byte(documentStart)...)
	withStart = append(withStart, bytes...)
	_, err = a.fileLogger.Write(withStart)
	return errors.Trace(err)
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
