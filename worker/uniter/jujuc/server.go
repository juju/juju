// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The worker/uniter/jujuc package implements the server side of the jujuc proxy
// tool, which forwards command invocations to the unit agent process so that
// they can be executed against specific state.
package jujuc

import (
	"bytes"
	"fmt"
	"net"
	"net/rpc"
	"path/filepath"
	"sort"
	"sync"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/utils/exec"
)

var logger = loggo.GetLogger("worker.uniter.jujuc")

// newCommands maps Command names to initializers.
var newCommands = map[string]func(Context) cmd.Command{
	"close-port":    NewClosePortCommand,
	"config-get":    NewConfigGetCommand,
	"juju-log":      NewJujuLogCommand,
	"open-port":     NewOpenPortCommand,
	"relation-get":  NewRelationGetCommand,
	"relation-ids":  NewRelationIdsCommand,
	"relation-list": NewRelationListCommand,
	"relation-set":  NewRelationSetCommand,
	"unit-get":      NewUnitGetCommand,
	"owner-get":     NewOwnerGetCommand,
}

// CommandNames returns the names of all jujuc commands.
func CommandNames() (names []string) {
	for name := range newCommands {
		names = append(names, name)
	}
	sort.Strings(names)
	return
}

// NewCommand returns an instance of the named Command, initialized to execute
// against the supplied Context.
func NewCommand(ctx Context, name string) (cmd.Command, error) {
	f := newCommands[name]
	if f == nil {
		return nil, fmt.Errorf("unknown command: %s", name)
	}
	return f(ctx), nil
}

// Request contains the information necessary to run a Command remotely.
type Request struct {
	ContextId   string
	Dir         string
	CommandName string
	Args        []string
}

// CmdGetter looks up a Command implementation connected to a particular Context.
type CmdGetter func(contextId, cmdName string) (cmd.Command, error)

// Jujuc implements the jujuc command in the form required by net/rpc.
type Jujuc struct {
	mu     sync.Mutex
	getCmd CmdGetter
}

// badReqErrorf returns an error indicating a bad Request.
func badReqErrorf(format string, v ...interface{}) error {
	return fmt.Errorf("bad request: "+format, v...)
}

// Main runs the Command specified by req, and fills in resp. A single command
// is run at a time.
func (j *Jujuc) Main(req Request, resp *exec.ExecResponse) error {
	if req.CommandName == "" {
		return badReqErrorf("command not specified")
	}
	if !filepath.IsAbs(req.Dir) {
		return badReqErrorf("Dir is not absolute")
	}
	c, err := j.getCmd(req.ContextId, req.CommandName)
	if err != nil {
		return badReqErrorf("%s", err)
	}
	var stdin, stdout, stderr bytes.Buffer
	ctx := &cmd.Context{
		Dir:    req.Dir,
		Stdin:  &stdin,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	logger.Infof("running hook tool %q %q", req.CommandName, req.Args)
	logger.Debugf("hook context id %q; dir %q", req.ContextId, req.Dir)
	resp.Code = cmd.Main(c, ctx, req.Args)
	resp.Stdout = stdout.Bytes()
	resp.Stderr = stderr.Bytes()
	return nil
}

// Server implements a server that serves command invocations via
// a unix domain socket.
type Server struct {
	socketPath string
	listener   net.Listener
	server     *rpc.Server
	closed     chan bool
	closing    chan bool
	wg         sync.WaitGroup
}

// NewServer creates an RPC server bound to socketPath, which can execute
// remote command invocations against an appropriate Context. It will not
// actually do so until Run is called.
func NewServer(getCmd CmdGetter, socketPath string) (*Server, error) {
	server := rpc.NewServer()
	if err := server.Register(&Jujuc{getCmd: getCmd}); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	s := &Server{
		socketPath: socketPath,
		listener:   listener,
		server:     server,
		closed:     make(chan bool),
		closing:    make(chan bool),
	}
	return s, nil
}

// Run accepts new connections until it encounters an error, or until Close is
// called, and then blocks until all existing connections have been closed.
func (s *Server) Run() (err error) {
	var conn net.Conn
	for {
		conn, err = s.listener.Accept()
		if err != nil {
			break
		}
		s.wg.Add(1)
		go func(conn net.Conn) {
			s.server.ServeConn(conn)
			s.wg.Done()
		}(conn)
	}
	select {
	case <-s.closing:
		// Someone has called Close(), so it is overwhelmingly likely that
		// the error from Accept is a direct result of the Listener being
		// closed, and can therefore be safely ignored.
		err = nil
	default:
	}
	s.wg.Wait()
	close(s.closed)
	return
}

// Close immediately stops accepting connections, and blocks until all existing
// connections have been closed.
func (s *Server) Close() {
	close(s.closing)
	s.listener.Close()
	<-s.closed
}
