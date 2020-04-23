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
	"syscall"
	"time"

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
		Upgrade            bool   `json:"upgrade"`
		CallerPID          int    `json:"caller-pid"`
	}

	socketName     string
	copyFunc       func(string, string) error
	symlinkFunc    func(string, string) error
	removeAllFunc  func(string) error
	mkdirAllFunc   func(string, os.FileMode) error
	statFunc       func(string) (os.FileInfo, error)
	listenFunc     func(sockets.Socket) (net.Listener, error)
	waitForPIDFunc func(int)
	stdErr         io.Writer
}

// NewCAASUnitInitCommand returns a new CAASUnitInitCommand that has been initialized.
func NewCAASUnitInitCommand() *CAASUnitInitCommand {
	return &CAASUnitInitCommand{
		socketName:     "@jujud-caas-unit-init",
		copyFunc:       copy,
		symlinkFunc:    os.Symlink,
		removeAllFunc:  os.RemoveAll,
		mkdirAllFunc:   os.MkdirAll,
		statFunc:       os.Stat,
		listenFunc:     sockets.Listen,
		waitForPIDFunc: waitForPID,
		stdErr:         os.Stderr,
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
	f.BoolVar(&c.Args.Upgrade, "upgrade", false, "upgrade only")
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
		defer func() {
			// When a sender sends a call to us, wait for them to exit
			// first, so when running under k8s, the container doesn't
			// get killed before the sender has died.
			c.waitForPIDFunc(c.Args.CallerPID)
		}()
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
		c.Args.CallerPID = os.Getpid()
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
	jujucPath := filepath.Join(tools.ToolsDir(cmdutil.DataDir, ""), "jujuc")
	// If jujuc doesn't exist use jujud
	if _, err = c.statFunc(jujucPath); os.IsNotExist(err) {
		jujucPath = jujudPath
	} else if err != nil {
		return errors.Annotatef(err, "failed to stat %s", jujucPath)
	}
	unitPaths := uniter.NewPaths(cmdutil.DataDir, unitTag, nil)
	if err = c.removeAllFunc(unitPaths.ToolsDir); err != nil && !os.IsNotExist(err) {
		return errors.Annotatef(err, "failed to remove unit tools dir %s",
			unitPaths.ToolsDir)
	}
	if err = c.mkdirAllFunc(unitPaths.ToolsDir, 0775); err != nil {
		return errors.Annotatef(err, "failed to make unit tools dir %s",
			unitPaths.ToolsDir)
	}
	if c.Args.Upgrade {
		if c.Args.CharmDir != "" {
			if err = c.removeAllFunc(unitPaths.State.CharmDir); err != nil && !os.IsNotExist(err) {
				return errors.Annotatef(err, "failed to remove unit charm dir %s",
					unitPaths.State.CharmDir)
			}
		}
	} else {
		if err = c.removeAllFunc(unitPaths.State.BaseDir); err != nil && !os.IsNotExist(err) {
			return errors.Annotatef(err, "failed to remove unit base dir %s",
				unitPaths.State.BaseDir)
		}
		if err = c.mkdirAllFunc(unitPaths.State.BaseDir, 0775); err != nil {
			return errors.Annotatef(err, "failed to make unit base dir %s",
				unitPaths.State.BaseDir)
		}
	}

	// symlink jujud
	ln := filepath.Join(unitPaths.ToolsDir, "jujud")
	logger.Infof("link %s => %s", ln, jujudPath)
	if err = c.symlinkFunc(jujudPath, ln); err != nil {
		return errors.Annotatef(err, "failed to link %s to %s",
			ln, jujudPath)
	}

	// symlink subcommands
	for _, cmdName := range jujuc.CommandNames() {
		ln := filepath.Join(unitPaths.ToolsDir, cmdName)
		logger.Infof("link %s => %s", ln, jujucPath)
		if err = c.symlinkFunc(jujucPath, ln); err != nil {
			return errors.Annotatef(err, "failed to link %s to %s",
				ln, jujucPath)
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

func waitForPID(pid int) {
	if pid == 0 || pid == os.Getpid() {
		return
	}
	// Check if the process exists. On Windows FindProcess would fail
	// if the process doesn't exist.
	// On UNIX kill with a 0 signal checks if the process exists
	// but doesn't send a signal.
	proc, err := os.FindProcess(pid)
	for err == nil && proc != nil {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			break
		}
		_ = proc.Release()
		time.Sleep(time.Second)
		proc, err = os.FindProcess(pid)
	}
}
