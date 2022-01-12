// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils/v2"
)

const readTimeout = 5 * time.Second

type replSession struct {
	id     string
	dbName string
	db     *sql.DB

	// The params for the current command and a writer for encoding the
	// command result.
	cmdParams string
	resWriter io.Writer
}

type replCmdDef struct {
	descr   string
	handler func(*replSession)
}

type sqlREPL struct {
	connListener net.Listener
	dbGetter     DBGetter
	clock        clock.Clock
	logger       Logger

	sessionCtx      context.Context
	sessionCancelFn func()
	sessionGroup    sync.WaitGroup

	commands map[string]replCmdDef
}

func newREPL(pathToSocket string, dbGetter DBGetter, clock clock.Clock, logger Logger) (REPL, error) {
	l, err := net.Listen("unix", pathToSocket)
	if err != nil {
		return nil, errors.Annotate(err, "creating UNIX socket for REPL sessions")
	}

	logger.Infof("serving REPL over %q", pathToSocket)

	ctx, cancelFn := context.WithCancel(context.TODO())

	r := &sqlREPL{
		connListener:    l,
		dbGetter:        dbGetter,
		clock:           clock,
		logger:          logger,
		sessionCtx:      ctx,
		sessionCancelFn: cancelFn,
	}
	r.registerCommands()
	go r.acceptConnections()

	return r, nil
}

// Kill implements the Worker interface. It closes the REPL socket and notifies
// any open sessions that they need to gracefully terminate.
func (r *sqlREPL) Kill() {
	r.logger.Infof("shutting down REPL socket and draining open sessions")
	r.connListener.Close()
}

// Wait implements the Worker interface. It blocks until all open REPL sessions
// terminate.
func (r *sqlREPL) Wait() error {
	r.sessionGroup.Wait()
	return nil
}

func (r *sqlREPL) registerCommands() {
	r.commands = map[string]replCmdDef{
		".help": {
			descr:   "display list of supported commands",
			handler: r.handleHelpCmd,
		},
		".open": {
			descr:   "connect to a database (e.g. '.open foo')",
			handler: r.handleOpenCommand,
		},
		".close": {
			descr:   "close connection to the current database",
			handler: r.handleCloseCommand,
		},
	}
}

func (r *sqlREPL) acceptConnections() {
	defer r.sessionGroup.Done()

	for {
		conn, err := r.connListener.Accept()
		if err != nil {
			return
		}

		r.sessionGroup.Add(1)
		go r.serveSession(conn)
	}
}

func (r *sqlREPL) serveSession(conn net.Conn) {
	sessionID, _ := utils.NewUUID()
	session := &replSession{
		id:        sessionID.String(),
		resWriter: conn,
	}

	defer func() {
		r.logger.Infof("[session: %v] terminating REPL session", session.id)
		r.sessionGroup.Done()
	}()
	r.logger.Infof("[session: %v] starting REPL session", session.id)

	// Render welcome banner and prompt
	_ = r.renderWelcomeBanner(conn)
	r.renderPrompt(conn, session)

	var cmdBuf = make([]byte, 4096)
	for {
		select {
		case <-r.sessionCtx.Done():
			// Make a best effort attempt to notify client that we are shutting down
			_, _ = fmt.Fprintf(conn, "\n*** REPL system is shutting down; terminating session\n")
			return
		default:
		}

		// Read input
		conn.SetReadDeadline(r.clock.Now().Add(readTimeout))
		n, err := conn.Read(cmdBuf)
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				continue // no command available
			}

			r.logger.Errorf("[session: %v] unable to read REPL command from client: %v", session.id, err)
			return
		} else if n == 0 {
			continue // no command available
		}

		// Process command and emit response
		r.processCommand(session, string(cmdBuf[:n]))

		// Render prompt
		r.renderPrompt(conn, session)
	}
}

func (r *sqlREPL) renderPrompt(w io.Writer, s *replSession) {
	if s.db == nil {
		_, _ = fmt.Fprintf(w, "\nrepl> ")
		return
	}
	_, _ = fmt.Fprintf(w, "\nrepl@%s> ", s.dbName)
}

func (r *sqlREPL) processCommand(s *replSession, input string) {
	tokens := strings.Fields(strings.TrimSpace(input))
	if len(tokens) == 0 {
		return
	}

	// Is it a known command?
	if cmd, known := r.commands[tokens[0]]; known {
		s.cmdParams = strings.Join(tokens[1:], " ")
		cmd.handler(s)
		return
	}

	// Is command a supported SQL statement type?
	switch {
	case strings.EqualFold(tokens[0], "SELECT"):
		s.cmdParams = strings.Join(tokens, " ")
		r.handleSQLSelectCommand(s)
	case strings.EqualFold(tokens[0], "INSERT"):
		fallthrough
	case strings.EqualFold(tokens[0], "UPDATE"):
		fallthrough
	case strings.EqualFold(tokens[0], "DELETE"):
		s.cmdParams = strings.Join(tokens, " ")
		r.handleSQLExecCommand(s)
	default:
		_, _ = fmt.Fprintf(s.resWriter, "Unknown command %q; for a list of supported commands type '.help'\n", tokens[0])
	}
}

func (r *sqlREPL) renderWelcomeBanner(w io.Writer) error {
	_, err := fmt.Fprintln(w, `
Welcome to the REPL for accessing dqlite databases for Juju models.

Before running any commands you must first connect to a database. To connect
to a database, type '.open' followed by the model UUID to connect to.

For a list of supported commands type '.help'`)

	return err
}

func (r *sqlREPL) handleHelpCmd(s *replSession) {
	cmdSet := set.NewStrings()
	for cmdName := range r.commands {
		cmdSet.Add(cmdName)
	}

	_, _ = fmt.Fprintf(s.resWriter, "The REPL supports the following commands:\n")

	for _, cmdName := range cmdSet.SortedValues() {
		_, _ = fmt.Fprintf(s.resWriter, "%s\t%s\n", cmdName, r.commands[cmdName].descr)
	}

	_, _ = fmt.Fprintf(s.resWriter, "\nIn addition, you can also type SQL SELECT statements\n")
}

func (r *sqlREPL) handleOpenCommand(s *replSession) {
	var err error

	s.db, err = r.dbGetter.GetExistingDB(s.cmdParams)
	if errors.IsNotFound(err) {
		_, _ = fmt.Fprintf(s.resWriter, "No such database exists\n")
		return
	} else if err != nil {
		r.logger.Errorf("[session: %v] unable to acquire DB handle: %v", s.id, err)
		_, _ = fmt.Fprintf(s.resWriter, "Unable to acquire DB handle; check the logs for more details\n")
		return
	}

	s.dbName = s.cmdParams
	_, _ = fmt.Fprintf(s.resWriter, "You are now connected to DB %q\n", s.cmdParams)
}

func (r *sqlREPL) handleCloseCommand(s *replSession) {
	if s.db == nil {
		_, _ = fmt.Fprintf(s.resWriter, "Not connected to a DB\n")
		return
	}

	_, _ = fmt.Fprintf(s.resWriter, "Disconnected from DB %q\n", s.dbName)
	s.db = nil
	s.dbName = ""
}

func (r *sqlREPL) handleSQLExecCommand(s *replSession) {
	if s.db == nil {
		_, _ = fmt.Fprintf(s.resWriter, "Not connected to a database; use '.open' followed by the model UUID to connect to\n")
		return
	}

	// NOTE(achilleasa): passing unfiltered user input to the DB is a
	// horrible horrible hack that should NEVER EVER see the light of day.
	// You have been warned!
	wrappedRes, err := withRetryWithResult(func() (interface{}, error) {
		return s.db.ExecContext(r.sessionCtx, s.cmdParams)
	})
	if err != nil {
		r.logger.Errorf("[session: %v] unable to execute query: %v", s.id, err)
		_, _ = fmt.Fprintf(s.resWriter, "Unable to execute query; check the logs for more details\n")
		return
	}
	res := wrappedRes.(sql.Result)

	lastInsertID, _ := res.LastInsertId()
	rowsAffected, _ := res.RowsAffected()
	_, _ = fmt.Fprintf(s.resWriter, "Affected Rows: %d; last insert ID: %v\n", rowsAffected, lastInsertID)
}

func (r *sqlREPL) handleSQLSelectCommand(s *replSession) {
	if s.db == nil {
		_, _ = fmt.Fprintf(s.resWriter, "Not connected to a database; use '.open' followed by the model UUID to connect to\n")
		return
	}

	// NOTE(achilleasa): passing unfiltered user input to the DB is a
	// horrible horrible hack that should NEVER EVER see the light of day.
	// You have been warned!
	wrappedRes, err := withRetryWithResult(func() (interface{}, error) {
		return s.db.QueryContext(r.sessionCtx, s.cmdParams)
	})
	if err != nil {
		r.logger.Errorf("[session: %v] unable to execute query: %v", s.id, err)
		_, _ = fmt.Fprintf(s.resWriter, "Unable to execute query; check the logs for more details\n")
		return
	}
	res := wrappedRes.(*sql.Rows)

	// Render header
	colMeta, err := res.Columns()
	if err != nil {
		r.logger.Errorf("[session: %v] unable to obtain column list for query: %v", s.id, err)
		_, _ = fmt.Fprintf(s.resWriter, "Unable to obtain column list for query; check the logs for more details\n")
		return
	}
	_, _ = fmt.Fprintf(s.resWriter, "%s\n", strings.Join(colMeta, "\t"))

	// Render Rows
	fieldList := make([]interface{}, len(colMeta))
	for i := 0; i < len(fieldList); i++ {
		var field interface{}
		fieldList[i] = &field
	}

	var rowCount int
	for res.Next() {
		rowCount++
		res.Scan(fieldList...)
		for i := 0; i < len(colMeta); i++ {
			var delim = '\t'
			if i == len(colMeta)-1 {
				delim = '\n'
			}

			field := *(fieldList[i].(*interface{}))
			_, _ = fmt.Fprintf(s.resWriter, "%v%c", field, delim)
		}
	}

	if err := res.Err(); err != nil {
		r.logger.Errorf("[session: %v] error while iterating query result set: %v", s.id, err)
		_, _ = fmt.Fprintf(s.resWriter, "Error while iterating query result set; check the logs for more details\n")
		return
	}

	_, _ = fmt.Fprintf(s.resWriter, "\nTotal rows: %d\n", rowCount)
}

const (
	maxDBRetries = 256
	retryDelay   = time.Millisecond
)

func withRetry(fn func() error) error {
	for attempt := 0; attempt < maxDBRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil // all done
		}

		// No point in re-trying or logging a no-row error.
		if errors.Cause(err) == sql.ErrNoRows {
			return sql.ErrNoRows
		}

		if !isRetriableError(err) {
			return err
		}

		jitter := time.Duration(rand.Float64() + 0.5)
		time.Sleep(retryDelay * jitter)
	}

	return errors.Errorf("unable to complete request after %d retries", maxDBRetries)
}

func withRetryWithResult(fn func() (interface{}, error)) (interface{}, error) {
	var (
		res interface{}
		err error
	)

	withRetry(func() error {
		res, err = fn()
		return err
	})

	return res, err
}

// isRetriableError returns true if the given error might be transient and the
// interaction can be safely retried.
func isRetriableError(err error) bool {
	err = errors.Cause(err)
	if err == nil {
		return false
	}

	if isDBAppError(err) {
		return true
	}

	if strings.Contains(err.Error(), "database is locked") {
		return true
	}

	if strings.Contains(err.Error(), "cannot start a transaction within a transaction") {
		return true
	}

	if strings.Contains(err.Error(), "bad connection") {
		return true
	}

	if strings.Contains(err.Error(), "checkpoint in progress") {
		return true
	}

	return false
}
