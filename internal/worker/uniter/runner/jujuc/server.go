// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3/exec"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/juju/sockets"
)

// This logger is fine being package level as this jujuc executable
// is separate from the uniter in that it is run inside hooks.
var logger = loggo.GetLogger("jujuc")

// ErrNoStdin is returned by Jujuc.Main if the hook tool requests
// stdin, and none is supplied.
var ErrNoStdin = errors.New("hook tool requires stdin, none supplied")

type creator func(Context) (cmd.Command, error)

// baseCommands maps Command names to creators.
var baseCommands = map[string]creator{
	"close-port":              NewClosePortCommand,
	"config-get":              NewConfigGetCommand,
	"juju-log":                NewJujuLogCommand,
	"open-port":               NewOpenPortCommand,
	"opened-ports":            NewOpenedPortsCommand,
	"relation-get":            NewRelationGetCommand,
	"relation-ids":            NewRelationIdsCommand,
	"relation-list":           NewRelationListCommand,
	"relation-model-get":      NewRelationModelGetCommand,
	"relation-set":            NewRelationSetCommand,
	"unit-get":                NewUnitGetCommand,
	"add-metric":              NewAddMetricCommand,
	"juju-reboot":             NewJujuRebootCommand,
	"status-get":              NewStatusGetCommand,
	"status-set":              NewStatusSetCommand,
	"network-get":             NewNetworkGetCommand,
	"application-version-set": NewApplicationVersionSetCommand,
	"k8s-spec-set":            constructCommandCreator("k8s-spec-set", NewK8sSpecSetCommand),
	"k8s-spec-get":            constructCommandCreator("k8s-spec-get", NewK8sSpecGetCommand),
	"k8s-raw-set":             NewK8sRawSetCommand,
	"k8s-raw-get":             NewK8sRawGetCommand,
	// "pod" variants are deprecated.
	"pod-spec-set": constructCommandCreator("pod-spec-set", NewK8sSpecSetCommand),
	"pod-spec-get": constructCommandCreator("pod-spec-get", NewK8sSpecGetCommand),

	"goal-state":     NewGoalStateCommand,
	"credential-get": NewCredentialGetCommand,

	"state-get":    NewStateGetCommand,
	"state-delete": NewStateDeleteCommand,
	"state-set":    NewStateSetCommand,
}

type functionCmdCreator func(Context, string) (cmd.Command, error)

func constructCommandCreator(name string, newCmd functionCmdCreator) creator {
	return func(ctx Context) (cmd.Command, error) {
		return newCmd(ctx, name)
	}
}

var secretCommands = map[string]creator{
	"secret-add":      NewSecretAddCommand,
	"secret-set":      NewSecretSetCommand,
	"secret-remove":   NewSecretRemoveCommand,
	"secret-get":      NewSecretGetCommand,
	"secret-info-get": NewSecretInfoGetCommand,
	"secret-grant":    NewSecretGrantCommand,
	"secret-revoke":   NewSecretRevokeCommand,
	"secret-ids":      NewSecretIdsCommand,
}

var storageCommands = map[string]creator{
	"storage-add":  NewStorageAddCommand,
	"storage-get":  NewStorageGetCommand,
	"storage-list": NewStorageListCommand,
}

var leaderCommands = map[string]creator{
	"is-leader":  NewIsLeaderCommand,
	"leader-get": NewLeaderGetCommand,
	"leader-set": NewLeaderSetCommand,
}

var resourceCommands = map[string]creator{
	"resource-get": NewResourceGetCmd,
}

var payloadCommands = map[string]creator{
	"payload-register":   NewPayloadRegisterCmd,
	"payload-unregister": NewPayloadUnregisterCmd,
	"payload-status-set": NewPayloadStatusSetCmd,
}

var actionCommands = map[string]creator{
	"action-get":  NewActionGetCommand,
	"action-set":  NewActionSetCommand,
	"action-fail": NewActionFailCommand,
	"action-log":  NewActionLogCommand,
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
	add(resourceCommands)
	add(payloadCommands)
	add(secretCommands)
	add(actionCommands)
	return all
}

func allHookCommands() map[string]creator {
	all := map[string]creator{}
	add := func(m map[string]creator) {
		for k, v := range m {
			all[k] = v
		}
	}
	add(baseCommands)
	add(storageCommands)
	add(leaderCommands)
	add(resourceCommands)
	add(payloadCommands)
	add(secretCommands)
	return all
}

func allActionCommands() map[string]creator {
	all := map[string]creator{}
	add := func(m map[string]creator) {
		for k, v := range m {
			all[k] = v
		}
	}

	add(actionCommands)
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

// ActionCommandNames returns the names of all jujuc action commands.
func ActionCommandNames() (names []string) {
	for name := range actionCommands {
		names = append(names, name)
	}
	sort.Strings(names)
	return
}

// HookCommandNames returns the names of all jujuc hook commands.
func HookCommandNames() (names []string) {
	for name := range allEnabledCommands() {
		if _, exists := actionCommands[name]; exists {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return
}

// NewHookCommand returns an instance of the named hook Command, initialized to execute
// against the supplied Context.
func NewHookCommand(ctx Context, name string) (cmd.Command, error) {
	f := allHookCommands()[name]
	if f == nil {
		return nil, errors.Errorf("unknown hook command: %s", name)
	}
	command, err := f(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return command, nil
}

// NewActionCommand returns an instance of the named action Command, initialized to execute
// against the supplied Context.
func NewActionCommand(ctx Context, name string) (cmd.Command, error) {
	f := allActionCommands()[name]
	if f == nil {
		return nil, errors.Errorf("unknown action command: %s", name)
	}
	command, err := f(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return command, nil
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

	Token string
}

// CmdGetter looks up a Command implementation connected to a particular Context.
type CmdGetter func(contextId, cmdName string) (cmd.Command, error)

// Jujuc implements the jujuc command in the form required by net/rpc.
type Jujuc struct {
	mu     sync.Mutex
	getCmd CmdGetter
	token  string
}

// badReqErrorf returns an error indicating a bad Request.
func badReqErrorf(format string, v ...interface{}) error {
	return fmt.Errorf("bad request: "+format, v...)
}

// Main runs the Command specified by req, and fills in resp. A single command
// is run at a time.
func (j *Jujuc) Main(req Request, resp *exec.ExecResponse) error {
	if req.Token != j.token {
		return badReqErrorf("token does not match")
	}
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
	logger.Debugf("running hook tool %q for %s", req.CommandName, req.ContextId)
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
	socket   sockets.Socket
	listener net.Listener
	server   *rpc.Server
	closed   chan bool
	closing  chan bool
	wg       sync.WaitGroup
}

// NewServer creates an RPC server bound to socketPath, which can execute
// remote command invocations against an appropriate Context. It will not
// actually do so until Run is called.
func NewServer(getCmd CmdGetter, socket sockets.Socket, token string) (*Server, error) {
	server := rpc.NewServer()
	if err := server.Register(&Jujuc{getCmd: getCmd, token: token}); err != nil {
		return nil, err
	}
	listener, err := sockets.Listen(socket)
	if err != nil {
		return nil, errors.Annotate(err, "listening to jujuc socket")
	}
	s := &Server{
		socket:   socket,
		listener: listener,
		server:   server,
		closed:   make(chan bool),
		closing:  make(chan bool),
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
	_ = os.Remove(s.socket.Address)
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

func (c *cmdWrapper) Info() *cmd.Info {
	return jujucmd.Info(c.Command.Info())
}
