// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

import (
	"context"
	"fmt"
	"hash/crc32"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v3/ssh"
	"golang.org/x/sync/errgroup"
)

type FileTransporter struct {
	params   ConfigureParams
	prefix   string
	safeHost string

	fileSends []file
}

type file struct {
	name     string
	payload  []byte
	destPath string
}

// NewFileTransporter returns an SCP file transporter that implements a cloudinit.FileTransporter
// to send payloads to the target machine, saving them in a temporary location.
func NewFileTransporter(params ConfigureParams) *FileTransporter {
	ft := &FileTransporter{
		params: params,
	}

	ft.prefix = "juju-" + strconv.Itoa(rand.Int())

	userHost := strings.SplitN(ft.params.Host, "@", 2)
	user := ""
	host := ""
	if len(userHost) == 1 {
		host = userHost[0]
	} else {
		user = userHost[0]
		host = userHost[1]
	}
	if strings.Contains(host, ":") {
		host = fmt.Sprintf("[%s]", host)
	}
	if user == "" {
		ft.safeHost = host
	} else {
		ft.safeHost = fmt.Sprintf("%s@%s", user, host)
	}
	return ft
}

// SendBytes implements cloudinit.FileTransporter
func (ft *FileTransporter) SendBytes(hint string, payload []byte) string {
	cs := crc32.ChecksumIEEE([]byte(hint))
	pathComponents := strings.Split(hint, "/")
	human := pathComponents[len(pathComponents)-1]
	name := fmt.Sprintf("%s-%x-%s", ft.prefix, cs, human)
	destPath := fmt.Sprintf("/tmp/%s", name)

	ft.fileSends = append(ft.fileSends, file{
		name:     name,
		payload:  payload,
		destPath: destPath,
	})

	return destPath
}

// Dispatch for all the files to finish transferring before returning.
func (ft *FileTransporter) Dispatch(ctx context.Context) error {
	eg, _ := errgroup.WithContext(ctx)

	for _, f := range ft.fileSends {
		f := f
		eg.Go(func() error {
			return ft.doCopy(f)
		})
	}
	ft.fileSends = nil

	return eg.Wait()
}

func (ft *FileTransporter) doCopy(f file) error {
	client := ft.params.Client
	if client == nil {
		client = ssh.DefaultClient
	}

	srcTmp := path.Join(os.TempDir(), f.name)
	err := os.WriteFile(srcTmp, f.payload, 0644)
	f.payload = nil
	if err != nil {
		return errors.Annotatef(err, "failed writing temp file %s", srcTmp)
	}
	defer os.Remove(srcTmp)

	dst := fmt.Sprintf("%s:%s", ft.safeHost, f.destPath)
	err = client.Copy([]string{srcTmp, dst}, ft.params.SSHOptions)
	if err != nil {
		return errors.Annotatef(err, "failed scp-ing file %s to %s", srcTmp, dst)
	}
	return nil
}
