// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/caas"
	jujucmd "github.com/juju/juju/cmd"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// CAASUnitInitCommand represents a jujud bootstrap command.
type CAASUnitInitCommand struct {
	cmd.CommandBase

	Wait bool
	Send bool
	Args struct {
		Unit               string `json:"unit"`
		OperatorFile       string `json:"operator-file"`
		OperatorCACertFile string `json:"operator-ca-cert-file"`
		CharmDir           string `json:"charm-dir"`
	}

	socketName    string
	copyFunc      func(string, string) error
	symlinkFunc   func(string, string) error
	removeAllFunc func(string) error
	mkdirAllFunc  func(string, os.FileMode) error
	listenFunc    func(sockets.Socket) (net.Listener, error)
	stdErr        io.Writer
}

// NewCAASUnitInitCommand returns a new CAASUnitInitCommand that has been initialized.
func NewCAASUnitInitCommand() *CAASUnitInitCommand {
	return &CAASUnitInitCommand{
		socketName:    "@jujud-caas-unit-init",
		copyFunc:      copy,
		symlinkFunc:   os.Symlink,
		removeAllFunc: os.RemoveAll,
		mkdirAllFunc:  os.MkdirAll,
		listenFunc:    sockets.Listen,
		stdErr:        os.Stderr,
	}
}

// Info returns a description of the command.
func (c *CAASUnitInitCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "caas-unit-init",
		Purpose: "initialize caas unit filesystem",
	})
}

func (c *CAASUnitInitCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Wait, "wait", false, "wait for args init via socket")
	f.BoolVar(&c.Send, "send", false, "send args for init via socket")
	f.StringVar(&c.Args.Unit, "unit", "", "unit name")
	f.StringVar(&c.Args.OperatorFile, "operator-file", "", "operator client info file")
	f.StringVar(&c.Args.OperatorCACertFile, "operator-ca-cert-file", "", "ca cert for operator")
	f.StringVar(&c.Args.CharmDir, "charm-dir", "", "directory containing the charm")
}

func (c *CAASUnitInitCommand) Init(args []string) error {
	if c.Wait && c.Send {
		return errors.New("only one or none of --wait/--send can be specified")
	}
	return nil
}

func (c *CAASUnitInitCommand) Run(ctx *cmd.Context) (errOut error) {
	sock := sockets.Socket{
		Network: "unix",
		Address: c.socketName,
	}
	// The waiting process waits for arguments to be sent by the
	// sending process. If neither wait or send is specified,
	// continue with the arguments already specified.
	if c.Wait {
		l, err := c.listenFunc(sock)
		if err != nil {
			return errors.Trace(err)
		}
		defer l.Close()
		conn, err := l.Accept()
		if err != nil {
			return errors.Trace(err)
		}
		defer conn.Close()
		err = json.NewDecoder(conn).Decode(&c.Args)
		if err != nil {
			return errors.Trace(err)
		}
		// Write logs back to the sender.
		w := loggo.NewSimpleWriter(conn, loggo.DefaultFormatter)
		err = loggo.RegisterWriter("socket", w)
		if err != nil {
			return errors.Trace(err)
		}
	} else if c.Send {
		conn, err := net.Dial(sock.Network, sock.Address)
		if err != nil {
			return errors.Trace(err)
		}
		err = json.NewEncoder(conn).Encode(c.Args)
		if err != nil {
			return errors.Trace(err)
		}
		// Read logs from the waiter.
		_, err = io.Copy(c.stdErr, conn)
		return err
	}

	defer func() {
		if errOut != nil {
			logger.Errorf("%v", errOut)
		}
	}()

	if c.Args.Unit == "" {
		return errors.New("missing unit arg")
	}

	unitTag, err := names.ParseUnitTag(c.Args.Unit)
	if err != nil {
		return errors.Trace(err)
	}

	jujudPath := filepath.Join(tools.ToolsDir(cmdutil.DataDir, ""), "jujud")
	unitPaths := uniter.NewPaths(cmdutil.DataDir, unitTag, nil)
	if err = c.removeAllFunc(unitPaths.ToolsDir); err != nil && !os.IsNotExist(err) {
		return errors.Annotatef(err, "failed to remove unit tools dir %s",
			unitPaths.ToolsDir)
	}
	if err = c.mkdirAllFunc(unitPaths.ToolsDir, 0775); err != nil {
		return errors.Annotatef(err, "failed to make unit tools dir %s",
			unitPaths.ToolsDir)
	}
	if err = c.removeAllFunc(unitPaths.State.BaseDir); err != nil && !os.IsNotExist(err) {
		return errors.Annotatef(err, "failed to remove unit base dir %s",
			unitPaths.State.BaseDir)
	}
	if err = c.mkdirAllFunc(unitPaths.State.BaseDir, 0775); err != nil {
		return errors.Annotatef(err, "failed to make unit base dir %s",
			unitPaths.State.BaseDir)
	}

	commandNames := append([]string{"jujud"}, jujuc.CommandNames()...)
	for _, cmdName := range commandNames {
		ln := filepath.Join(unitPaths.ToolsDir, cmdName)
		logger.Infof("link %s => %s", ln, jujudPath)
		if err = c.symlinkFunc(jujudPath, ln); err != nil {
			return errors.Annotatef(err, "failed to link %s to %s",
				ln, jujudPath)
		}
	}

	var copies []srcDest
	if c.Args.OperatorFile != "" {
		operatorFile := filepath.Join(unitPaths.State.BaseDir, caas.OperatorClientInfoFile)
		copies = append(copies, srcDest{c.Args.OperatorFile, operatorFile})
	}
	if c.Args.OperatorCACertFile != "" {
		caCertFile := filepath.Join(unitPaths.State.BaseDir, caas.CACertFile)
		copies = append(copies, srcDest{c.Args.OperatorCACertFile, caCertFile})
	}
	if c.Args.CharmDir != "" {
		copies = append(copies, srcDest{c.Args.CharmDir, unitPaths.State.CharmDir})
	}

	for _, op := range copies {
		logger.Infof("copy %s => %s", op.src, op.dst)
		if err = c.copyFunc(op.src, op.dst); err != nil {
			return errors.Annotatef(err, "failed to copy %s to %s", op.src, op.dst)
		}
	}

	return nil
}

func copy(src, dst string) error {
	logger.Infof("copy %s => %s", src, dst)
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cp -Rf %q %q", src, dst))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Annotatef(err, "output: %s", string(out))
	}
	return nil
}

type srcDest struct {
	src string
	dst string
}
