// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The worker/uniter/runner/jujuc package implements the server side of the
// jujuc proxy tool, which forwards command invocations to the unit agent
// process so that they can be executed against specific state.
package jujuc

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sort"
	"sync"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

const (
	StdioRpcType       = "Stdio"
	ReadStdinRpcAction = "ReadStdin"
)

var logger = loggo.GetLogger("worker.uniter.jujuc")

type creator func(Context) cmd.Command

// baseCommands maps Command names to creators.
var baseCommands = map[string]creator{
	"close-port" + cmdSuffix:    NewClosePortCommand,
	"config-get" + cmdSuffix:    NewConfigGetCommand,
	"juju-log" + cmdSuffix:      NewJujuLogCommand,
	"open-port" + cmdSuffix:     NewOpenPortCommand,
	"opened-ports" + cmdSuffix:  NewOpenedPortsCommand,
	"relation-get" + cmdSuffix:  NewRelationGetCommand,
	"action-get" + cmdSuffix:    NewActionGetCommand,
	"action-set" + cmdSuffix:    NewActionSetCommand,
	"action-fail" + cmdSuffix:   NewActionFailCommand,
	"relation-ids" + cmdSuffix:  NewRelationIdsCommand,
	"relation-list" + cmdSuffix: NewRelationListCommand,
	"relation-set" + cmdSuffix:  NewRelationSetCommand,
	"unit-get" + cmdSuffix:      NewUnitGetCommand,
	"owner-get" + cmdSuffix:     NewOwnerGetCommand,
	"add-metric" + cmdSuffix:    NewAddMetricCommand,
	"juju-reboot" + cmdSuffix:   NewJujuRebootCommand,
	"status-get" + cmdSuffix:    NewStatusGetCommand,
	"status-set" + cmdSuffix:    NewStatusSetCommand,
}

var storageCommands = map[string]creator{
	"storage-add" + cmdSuffix: NewStorageAddCommand,
	"storage-get" + cmdSuffix: NewStorageGetCommand,
}

var leaderCommands = map[string]creator{
	"is-leader" + cmdSuffix:  NewIsLeaderCommand,
	"leader-get" + cmdSuffix: NewLeaderGetCommand,
	"leader-set" + cmdSuffix: NewLeaderSetCommand,
}

func allEnabledCommands() map[string]creator {
	all := map[string]creator{}
	add := func(m map[string]creator) {
		for k, v := range m {
			all[k] = v
		}
	}
	add(baseCommands)
	add(storageCommands)
	add(leaderCommands)
	return all
}

// CommandNames returns the names of all jujuc commands.
func CommandNames() (names []string) {
	for name := range allEnabledCommands() {
		names = append(names, name)
	}
	sort.Strings(names)
	return
}

// NewCommand returns an instance of the named Command, initialized to execute
// against the supplied Context.
func NewCommand(ctx Context, name string) (cmd.Command, error) {
	f := allEnabledCommands()[name]
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

// rpcRoot is the RPC root for handling jujuc commands.
type rpcRoot struct {
	requestMutex *sync.Mutex
	getCmd       CmdGetter
	stdio        *StdioClient
}

func (r *rpcRoot) Jujuc(id string) (*Jujuc, error) {
	return &Jujuc{r.requestMutex, r.getCmd, r.stdio.Stdin()}, nil
}

// Jujuc implements the jujuc command in the form required by juju/rpc.
type Jujuc struct {
	requestMutex *sync.Mutex
	getCmd       CmdGetter
	stdin        io.Reader
}

// badReqErrorf returns an error indicating a bad Request.
func badReqErrorf(format string, v ...interface{}) error {
	return fmt.Errorf("bad request: "+format, v...)
}

// Main runs the Command specified by req, and fills in resp. A single command
// is run at a time.
func (j *Jujuc) Main(req Request) (exec.ExecResponse, error) {
	if req.CommandName == "" {
		return exec.ExecResponse{}, badReqErrorf("command not specified")
	}
	if !filepath.IsAbs(req.Dir) {
		return exec.ExecResponse{}, badReqErrorf("Dir is not absolute")
	}
	c, err := j.getCmd(req.ContextId, req.CommandName)
	if err != nil {
		return exec.ExecResponse{}, badReqErrorf("%s", err)
	}
	var stdout, stderr bytes.Buffer
	ctx := &cmd.Context{
		Dir:    req.Dir,
		Stdin:  j.stdin,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	j.requestMutex.Lock()
	defer j.requestMutex.Unlock()
	logger.Infof("running hook tool %q %q", req.CommandName, req.Args)
	logger.Debugf("hook context id %q; dir %q", req.ContextId, req.Dir)
	resp := exec.ExecResponse{
		Code:   cmd.Main(c, ctx, req.Args),
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}
	return resp, nil
}

// Server implements a server that serves command invocations via
// a unix domain socket.
type Server struct {
	socketPath string
	listener   net.Listener
	getCmd     CmdGetter
	closed     chan bool
	closing    chan bool
	wg         sync.WaitGroup
	// requestMutex is shared by all rpcRoots to serialise requests.
	requestMutex sync.Mutex
}

// NewServer creates an RPC server bound to socketPath, which can execute
// remote command invocations against an appropriate Context. It will not
// actually do so until Run is called.
func NewServer(getCmd CmdGetter, socketPath string) (*Server, error) {
	listener, err := sockets.Listen(socketPath)
	if err != nil {
		return nil, err
	}
	s := &Server{
		socketPath: socketPath,
		listener:   listener,
		getCmd:     getCmd,
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
		go func(netConn net.Conn) {
			defer s.wg.Done()
			codec := jsoncodec.NewNet(netConn)
			conn := rpc.NewConn(codec, nil)
			stdio := &StdioClient{conn}
			conn.Serve(&rpcRoot{&s.requestMutex, s.getCmd, stdio}, nil)
			conn.Start()
			select {
			case <-s.closing:
				conn.Close()
			case <-conn.Dead():
			}
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
	return err
}

// Close immediately stops accepting connections, and blocks until all existing
// connections have been closed.
func (s *Server) Close() {
	close(s.closing)
	s.listener.Close()
	<-s.closed
}
