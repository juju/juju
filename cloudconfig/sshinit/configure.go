// Copyright 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/ssh"
	"golang.org/x/sync/errgroup"

	"github.com/juju/juju/cloudconfig/cloudinit"
)

var logger = loggo.GetLogger("juju.cloudinit.sshinit")

type ConfigureParams struct {
	// Host is the host to configure, in the format [user@]hostname.
	Host string

	// Client is the SSH client to connect with.
	// If Client is nil, ssh.DefaultClient will be used.
	Client ssh.Client

	// SSHOptions contains options for running the SSH command.
	SSHOptions *ssh.Options

	// Config is the cloudinit config to carry out.
	Config cloudinit.CloudConfig

	// ProgressWriter is an io.Writer to which progress will be written,
	// for realtime feedback.
	ProgressWriter io.Writer

	// OS is the os of the machine on which the script will be carried out
	OS string
}

// RunConfigureScript connects to the specified host over
// SSH, and executes the provided script which is expected
// to have been returned by cloudinit ConfigureScript.
func RunConfigureScript(script string, params ConfigureParams) error {
	logger.Tracef("Running script on %s: %s", params.Host, script)

	encoded := base64.StdEncoding.EncodeToString([]byte(`
set -e
tmpfile=$(mktemp)
trap "rm -f $tmpfile" EXIT
cat > $tmpfile
/bin/bash $tmpfile
`))

	client := params.Client
	if client == nil {
		client = ssh.DefaultClient
	}

	// bash will read a byte at a time when consuming commands
	// from stdin. We avoid sending the entire script -- which
	// will be very large when uploading tools -- directly to
	// bash for this reason. Instead, run cat which will write
	// the script to disk, and then execute it from there.
	cmd := client.Command(params.Host, []string{
		"sudo", "/bin/bash", "-c",
		// The outer bash interprets the $(...), and executes
		// the decoded script in the nested bash. This avoids
		// linebreaks in the commandline, which the go.crypto-
		// based client has trouble with.
		fmt.Sprintf(
			`/bin/bash -c "$(echo %s | base64 -d)"`,
			utils.ShQuote(encoded),
		),
	}, params.SSHOptions)

	cmd.Stdin = strings.NewReader(script)
	cmd.Stderr = params.ProgressWriter
	return cmd.Run()
}

type FileTransporter struct {
	params   ConfigureParams
	prefix   string
	safeHost string

	errGroup *errgroup.Group
	ctx      context.Context
}

func NewFileTransporter(ctx context.Context, params ConfigureParams) *FileTransporter {
	ft := &FileTransporter{
		params: params,
	}
	eg, egCtx := errgroup.WithContext(ctx)
	ft.errGroup = eg
	ft.ctx = egCtx
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

func (ft *FileTransporter) SendBytes(hint string, payload []byte) string {
	cs := crc32.ChecksumIEEE([]byte(hint))
	pathComponents := strings.Split(hint, "/")
	human := pathComponents[len(pathComponents)-1]
	name := fmt.Sprintf("%s-%x-%s", ft.prefix, cs, human)
	dstTmp := fmt.Sprintf("/tmp/%s", name)
	ft.errGroup.Go(func() error {
		client := ft.params.Client
		if client == nil {
			client = ssh.DefaultClient
		}

		srcTmp := path.Join(os.TempDir(), name)
		err := ioutil.WriteFile(srcTmp, payload, 0644)
		if err != nil {
			return errors.Annotatef(err, "failed writing temp file %s", srcTmp)
		}
		defer os.Remove(srcTmp)

		dst := fmt.Sprintf("%s:%s", ft.safeHost, dstTmp)
		err = client.Copy([]string{srcTmp, dst}, ft.params.SSHOptions)
		if err != nil {
			return errors.Annotatef(err, "failed scp-ing file %s to %s", srcTmp, dst)
		}
		return nil
	})
	return dstTmp
}

func (ft *FileTransporter) Wait() error {
	return ft.errGroup.Wait()
}
