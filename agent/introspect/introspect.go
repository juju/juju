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
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"
	"github.com/kr/pretty"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	"github.com/juju/juju/agent/config"
	jujucmd "github.com/juju/juju/cmd"
	coreagent "github.com/juju/juju/core/agent"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/juju/sockets"
)

type IntrospectCommand struct {
	cmd.CommandBase
	dataDir string
	agent   string
	path    string
	listen  string

	verbose bool
	post    bool
	form    url.Values
}

// New initializes IntrospectCommand.
func New() cmd.Command {
	return &IntrospectCommand{}
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
		Args:    "(--listen=...|<path> [key=value [...]])",
		Purpose: "introspect Juju agents running on this machine",
		Doc:     introspectCommandDoc,
	})
}

func (c *IntrospectCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.dataDir, "data-dir", config.DataDir, "Juju base data directory")
	f.StringVar(&c.agent, "agent", "", "agent to introspect (defaults to machine agent)")
	f.StringVar(&c.listen, "listen", "", "address on which to expose the introspection socket")
	f.BoolVar(&c.post, "post", false, "perform a POST action rather than a GET")
	f.BoolVar(&c.verbose, "verbose", false, "show query path and args")
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
	if c.post == false {
		// No args expected for post.
		return c.CommandBase.Init(args)
	}
	// Additional args are expected to be "key=value", and are added
	// to url form arguments.
	c.form = url.Values{}
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) < 2 {
			return errors.New("form value missing '='")
		}
		key, value := parts[0], parts[1]
		c.form[key] = append(c.form[key], value)
	}
	return nil
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

	socketName := path.Join(agent.Dir(c.dataDir, tag), addons.IntrospectionSocketName)
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

	client := unixSocketHTTPClient(socketName)
	var resp *http.Response
	if c.post {
		if c.verbose {
			ctx.Infof("Posting to %s introspection socket: %s %s", socketName, c.path, pretty.Sprint(c.form))
		}
		resp, err = client.PostForm(targetURL.String(), c.form)
	} else {
		if c.verbose {
			ctx.Infof("Querying %s introspection socket: %s", socketName, c.path)
		}
		resp, err = client.Get(targetURL.String())
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(ctx.Stderr, resp.Body)
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

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil, errors.Annotate(err, "reading agents dir")
	}
	for _, info := range entries {
		name := info.Name()
		tag, err := names.ParseTag(name)
		if err != nil {
			continue
		}
		if coreagent.IsAllowedControllerTag(tag.Kind()) {
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
			return sockets.Dialer(sockets.Socket{
				Network: "unix",
				Address: socketPath,
			})
		},
	}
}
