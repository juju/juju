// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This package implements the server side of the
// hook tool CLI, which forwards command invocations to the CAAS operator
// process so that they can be executed in the Juju controller.
package commands

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"sync"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/worker/common/hooks"
	"github.com/juju/loggo"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/juju/sockets"
)

var logger = loggo.GetLogger("worker.caasoperator.commands")

// ErrNoStdin is returned by HookCommand.Main if the hook tool requests
// stdin, and none is supplied.
var ErrNoStdin = errors.New("hook tool requires stdin, none supplied")

// NewCommand returns an instance of the named Command, initialized to execute
// against the supplied Context.
func NewCommand(ctx hooks.Context, name string) (cmd.Command, error) {
	return hooks.NewCommand(ctx, name, allEnabledCommands)
}

// Request contains the information necessary to run a Command remotely.
type Request struct {
	ContextId   string
	Dir         string
	CommandName string
	Args        []string

	// StdinSet indicates whether or not the client supplied stdin. This is
	// necessary as Stdin will be nil if the client supplied stdin but it
	// is empty.
	StdinSet bool
	Stdin    []byte
}

// CmdGetter looks up a Command implementation connected to a particular Context.
type CmdGetter func(contextId, cmdName string) (cmd.Command, error)

// HookCommand implements the hook command in the form required by net/rpc.
type HookCommand struct {
	mu     sync.Mutex
	getCmd CmdGetter
}

// badReqErrorf returns an error indicating a bad Request.
func badReqErrorf(format string, v ...interface{}) error {
	return fmt.Errorf("bad request: "+format, v...)
}

// Main runs the Command specified by req, and fills in resp. A single command
// is run at a time.
func (j *HookCommand) Main(req Request, resp *exec.ExecResponse) error {
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
	var stdin io.Reader
	if req.StdinSet {
		stdin = bytes.NewReader(req.Stdin)
	} else {
		// noStdinReader will error with ErrNoStdin
		// if its Read method is called.
		stdin = noStdinReader{}
	}
	var stdout, stderr bytes.Buffer
	ctx := &cmd.Context{
		Dir:    req.Dir,
		Stdin:  stdin,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	// Beware, reducing the log level of the following line will lead
	// to passwords leaking if passed as args.
	logger.Tracef("running hook tool %q %q", req.CommandName, req.Args)
	logger.Debugf("running hook tool %q", req.CommandName)
	logger.Tracef("hook context id %q; dir %q", req.ContextId, req.Dir)
	wrapper := &cmdWrapper{c, nil}
	resp.Code = cmd.Main(wrapper, ctx, req.Args)
	if errors.Cause(wrapper.err) == ErrNoStdin {
		return ErrNoStdin
	}
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
	if err := server.Register(&HookCommand{getCmd: getCmd}); err != nil {
		return nil, err
	}
	listener, err := sockets.Listen(socketPath)
	if err != nil {
		return nil, errors.Annotate(err, "listening to hook command socket")
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
	// We need to remove the socket path because
	// we renamed the path after opening the
	// socket and it won't be cleaned up automatically.
	// Ignore error as we can't do much here
	// anyway and remove the path if we start the
	// server again.
	os.Remove(s.socketPath)
	<-s.closed
}

type noStdinReader struct{}

// Read implements io.Reader, simply returning ErrNoStdin any time it's called.
func (noStdinReader) Read([]byte) (int, error) {
	return 0, ErrNoStdin
}

// cmdWrapper wraps a cmd.Command's Run method so the error returned can be
// intercepted when the command is run via cmd.Main.
type cmdWrapper struct {
	cmd.Command
	err error
}

func (c *cmdWrapper) Run(ctx *cmd.Context) error {
	c.err = c.Command.Run(ctx)
	return c.err
}
