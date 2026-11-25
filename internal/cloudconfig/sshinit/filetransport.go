// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"
	"golang.org/x/sync/errgroup"
)

// FileTransporter stages cloud-init files on a machine before setup runs.
type FileTransporter struct {
	params ConfigureParams
	dir    string

	fileSends  []file
	nextIndex  int
	dispatched bool
}

type file struct {
	payload  []byte
	source   string
	fromFile bool
	destPath string
}

// NewFileTransporter returns a transporter for files needed by cloud-init.
// Its SSH options must not request a PTY, which would corrupt binary streams.
func NewFileTransporter(params ConfigureParams) (*FileTransporter, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, errors.Annotate(err, "creating a file staging name")
	}
	return &FileTransporter{
		params: params,
		dir:    path.Join("/tmp", "juju-"+hex.EncodeToString(token[:])),
	}, nil
}

// SendBytes implements cloudinit.FileTransporter.
func (ft *FileTransporter) SendBytes(hint string, payload []byte) string {
	return ft.addFile(hint, file{payload: payload})
}

// SendFile schedules a local file for transfer without reading it into memory.
// The source must remain available until Dispatch returns.
func (ft *FileTransporter) SendFile(hint, source string) string {
	return ft.addFile(hint, file{source: source, fromFile: true})
}

func (ft *FileTransporter) addFile(hint string, f file) string {
	human := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, path.Base(hint))
	if len(human) > 80 {
		human = human[:80]
	}
	name := fmt.Sprintf("%d-%x-%s", ft.nextIndex,
		crc32.ChecksumIEEE([]byte(hint)), human)
	ft.nextIndex++
	f.destPath = path.Join(ft.dir, name)
	ft.fileSends = append(ft.fileSends, f)
	return f.destPath
}

// Dispatch transfers all scheduled files. Call Cleanup afterwards, including
// on failure. Context cancellation is checked between commands; it does not
// interrupt a command that is already running.
func (ft *FileTransporter) Dispatch(ctx context.Context) error {
	if len(ft.fileSends) == 0 {
		return errors.Trace(ctx.Err())
	}
	if !ft.dispatched {
		client := ft.params.Client
		if client == nil {
			client = ssh.DefaultClient
		}
		cmd := client.Command(ft.params.Host, []string{
			"mkdir", "-m", "0700", "--", utils.ShQuote(ft.dir),
		}, ft.params.SSHOptions)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := runCommand(ctx, cmd); err != nil {
			if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
				return errors.Annotatef(err, "creating remote staging directory: %s", diagnostic)
			}
			return errors.Annotate(err, "creating remote staging directory")
		}
		ft.dispatched = true
	}
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(8)
	for _, f := range ft.fileSends {
		eg.Go(func() error {
			return ft.send(ctx, f)
		})
	}
	ft.fileSends = nil
	return eg.Wait()
}

func (ft *FileTransporter) send(ctx context.Context, f file) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var source io.Reader = bytes.NewReader(f.payload)
	if f.fromFile {
		file, err := os.Open(f.source)
		if err != nil {
			return errors.Annotatef(err, "opening file %q for transfer", f.source)
		}
		defer file.Close()
		source = file
	}
	client := ft.params.Client
	if client == nil {
		client = ssh.DefaultClient
	}
	// The temporary file must be private even if the destination is not.
	// noclobber prevents following or replacing an existing path in /tmp.
	cmd := client.Command(ft.params.Host, []string{
		"umask", "077;", "set", "-C;", "cat", ">", utils.ShQuote(f.destPath),
	}, ft.params.SSHOptions)
	cmd.Stdin = source
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := runCommand(ctx, cmd); err != nil {
		if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
			return errors.Annotatef(err, "transferring %q: %s", f.destPath, diagnostic)
		}
		return errors.Annotatef(err, "transferring %q", f.destPath)
	}
	return nil
}

// CleanupCommand returns the command that removes the staging directory. It
// is empty when no staging directory was created, and can run in the remote
// configuration script's EXIT trap to avoid reconnecting after host key rotation.
func (ft *FileTransporter) CleanupCommand() string {
	if !ft.dispatched {
		return ""
	}
	return "rm -rf -- " + utils.ShQuote(ft.dir)
}

// Cleanup removes the temporary remote files. The caller should provide a
// separately bounded context when the original bootstrap was cancelled.
func (ft *FileTransporter) Cleanup(ctx context.Context) error {
	if !ft.dispatched {
		return nil
	}
	client := ft.params.Client
	if client == nil {
		client = ssh.DefaultClient
	}
	args := []string{"rm", "-rf", "--", utils.ShQuote(ft.dir)}
	cmd := client.Command(ft.params.Host, args, ft.params.SSHOptions)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := runCommand(ctx, cmd); err != nil {
		if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
			return errors.Annotatef(err, "removing staged files: %s", diagnostic)
		}
		return errors.Annotate(err, "removing staged files")
	}
	ft.dispatched = false
	return nil
}

// runCommand executes cmd synchronously. The SSH library does not expose a
// safe way to force-close both of its backends, so context is only checked
// before and after the command rather than racing its termination.
func runCommand(ctx context.Context, cmd *ssh.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	return ctx.Err()
}
