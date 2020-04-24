// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspect

import (
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	apiagent "github.com/juju/juju/api/agent"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/jujud/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
)

type IntrospectCommand struct {
	cmd.CommandBase
	dataDir string
	agent   string
	path    string
	listen  string

	// IntrospectionSocketName returns the socket name
	// for a given tag. If IntrospectionSocketName is nil,
	// agent.DefaultIntrospectionSocketName is used.
	IntrospectionSocketName func(names.Tag) string
}

const introspectCommandDoc = `
Introspect Juju agents running on this machine.

The juju-introspect command can be used to expose
the agent's introspection socket via HTTP, using
the --listen flag. e.g.

    juju-introspect --listen=:6060

Otherwise, a single positional argument is required,
which is the path to query. e.g.

    juju-introspect /debug/pprof/heap?debug=1

By default, juju-introspect operates on the
machine agent. If you wish to introspect a
unit agent on the machine, you can specify the
agent using --agent. e.g.

    juju-introspect --agent=unit-mysql-0 metrics
`

// Info returns usage information for the command.
func (c *IntrospectCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "juju-introspect",
		Args:    "(--listen=...|<path>)",
		Purpose: "introspect Juju agents running on this machine",
		Doc:     introspectCommandDoc,
	})
}

func (c *IntrospectCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.dataDir, "data-dir", cmdutil.DataDir, "Juju base data directory")
	f.StringVar(&c.agent, "agent", "", "agent to introspect (defaults to machine agent)")
	f.StringVar(&c.listen, "listen", "", "address on which to expose the introspection socket")
}

func (c *IntrospectCommand) Init(args []string) error {
	if len(args) >= 1 {
		c.path, args = args[0], args[1:]
	}
	if c.path == "" && c.listen == "" {
		return errors.New("either a query path or a --listen address must be specified")
	}
	if c.path != "" && c.listen != "" {
		return errors.New("a query path may not be specified with --listen")
	}
	return c.CommandBase.Init(args)
}

func (c *IntrospectCommand) Run(ctx *cmd.Context) error {
	targetURL, err := url.Parse("http://unix.socket/" + c.path)
	if err != nil {
		return err
	}

	tag, err := c.getAgentTag()
	if err != nil {
		return err
	}

	getSocketName := c.IntrospectionSocketName
	if getSocketName == nil {
		getSocketName = agent.DefaultIntrospectionSocketName
	}
	socketName := "@" + getSocketName(tag)
	if c.listen != "" {
		listener, err := net.Listen("tcp", c.listen)
		if err != nil {
			return err
		}
		defer listener.Close()
		ctx.Infof("Exposing %s introspection socket on %s", socketName, listener.Addr())
		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.Transport = unixSocketHTTPTransport(socketName)
		return http.Serve(listener, proxy)
	}

	ctx.Infof("Querying %s introspection socket: %s", socketName, c.path)
	client := unixSocketHTTPClient(socketName)
	resp, err := client.Get(targetURL.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(ctx.Stderr, resp.Body)
		return errors.Errorf(
			"response returned %d (%s)",
			resp.StatusCode,
			http.StatusText(resp.StatusCode),
		)
	}
	_, err = io.Copy(ctx.Stdout, resp.Body)
	return err
}

func (c *IntrospectCommand) getAgentTag() (names.Tag, error) {
	if c.agent != "" {
		return names.ParseTag(c.agent)
	}
	agentsDir := filepath.Join(c.dataDir, "agents")
	dir, err := os.Open(agentsDir)
	if err != nil {
		return nil, errors.Annotate(err, "opening agents dir")
	}
	defer dir.Close()

	entries, err := dir.Readdir(-1)
	if err != nil {
		return nil, errors.Annotate(err, "reading agents dir")
	}
	for _, info := range entries {
		name := info.Name()
		tag, err := names.ParseTag(name)
		if err != nil {
			continue
		}
		if apiagent.IsAllowedControllerTag(tag.Kind()) {
			return tag, nil
		}
	}
	return nil, errors.New("could not determine machine or controller agent tag")
}

func unixSocketHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: unixSocketHTTPTransport(socketPath),
	}
}

func unixSocketHTTPTransport(socketPath string) *http.Transport {
	return &http.Transport{
		Dial: func(proto, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
}
