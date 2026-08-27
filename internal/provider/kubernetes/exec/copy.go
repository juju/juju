// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/filecopy"
)

// FileResource holds all the necessary parameters for the source or destination of a copy request.
type FileResource struct {
	Path          string
	PodName       string
	ContainerName string
}

func (cp *FileResource) validate() (err error) {
	if cp.Path == "" {
		return errors.New("path was missing")
	}
	return nil
}

// CopyParams holds all the necessary parameters for a copy request.
type CopyParams struct {
	Src                           FileResource
	Dest                          FileResource
	overwriteOwnershipPermissions bool
}

func (cp *CopyParams) validate() error {
	if err := cp.Src.validate(); err != nil {
		return errors.Trace(err)
	}
	if err := cp.Dest.validate(); err != nil {
		return errors.Trace(err)
	}
	if cp.Src.PodName != "" && cp.Dest.PodName != "" {
		return errors.New("cross pods copy is not supported")
	}
	if cp.Src.PodName == "" && cp.Dest.PodName == "" {
		return errors.New("copy either from pod to host or from host to pod")
	}
	return nil
}

// Copy copies files/directories from host to a pod or from a pod to host.
func (c client) Copy(ctx context.Context, params CopyParams, cancel <-chan struct{}) error {
	if err := params.validate(); err != nil {
		return errors.Trace(err)
	}
	if params.Src.PodName != "" {
		return c.copyFromPod(ctx, params, cancel)
	}
	if params.Dest.PodName != "" {
		return c.copyToPod(ctx, params, cancel)
	}
	return errors.NewNotValid(nil, "either copy from a pod or to a pod")
}

func (c client) copyFromPod(ctx context.Context, params CopyParams, cancel <-chan struct{}) error {
	src := params.Src
	dest := params.Dest
	logger.Debugf(context.TODO(), "copying from %v to %v", src, dest)

	reader, writer := c.pipGetter()
	var stderr bytes.Buffer
	execParams := ExecParams{
		PodName:       src.PodName,
		ContainerName: src.ContainerName,
		Commands:      []string{"tar", "cf", "-", src.Path},
		Stdin:         nil,
		Stdout:        writer,
		Stderr:        &stderr,
	}

	go func() {
		defer writer.Close()
		if err := c.Exec(ctx, execParams, cancel); err != nil {
			logger.Errorf(context.TODO(), "make tar %q failed: %v", src.Path, err)
		}
	}()
	return filecopy.UntarAll(src.Path, reader, dest.Path)
}

// this is inspired by kubectl cmd package.
// - https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/cp/cp.go
func (c client) copyToPod(ctx context.Context, params CopyParams, cancel <-chan struct{}) (err error) {
	src := params.Src
	dest := params.Dest
	logger.Debugf(context.TODO(), "copying from %v to %v", src, dest)

	if _, err = os.Stat(src.Path); err != nil {
		return errors.NewNotValid(nil, fmt.Sprintf("%q does not exist on local", src.Path))
	}

	if dest.Path != "/" && strings.HasSuffix(dest.Path, "/") {
		dest.Path = strings.TrimSuffix(dest.Path, "/")
	}

	if err = c.checkRemotePathIsDir(ctx, dest, cancel); err == nil {
		dest.Path = path.Join(dest.Path, path.Base(src.Path))
	}

	reader, writer := c.pipGetter()

	go func() {
		defer writer.Close()

		if err := filecopy.MakeTar(src.Path, dest.Path, writer); err != nil {
			logger.Errorf(context.TODO(), "make tar %q failed: %v", src.Path, err)
		}
	}()

	cmds := []string{"tar", "-xmf", "-"}
	if params.overwriteOwnershipPermissions {
		cmds = []string{"tar", "--no-same-permissions", "--no-same-owner", "-xmf", "-"}
	}
	destDir := path.Dir(dest.Path)
	if len(destDir) > 0 {
		cmds = append(cmds, "-C", destDir)
	}
	var stdout, stderr bytes.Buffer
	execParams := ExecParams{
		PodName:       dest.PodName,
		ContainerName: dest.ContainerName,
		Commands:      cmds,
		Stdin:         reader,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}
	return errors.Trace(c.Exec(ctx, execParams, cancel))
}

func (c client) checkRemotePathIsDir(ctx context.Context, rec FileResource, cancel <-chan struct{}) error {
	if rec.PodName == "" {
		return errors.NotValidf("empty pod name")
	}
	var stdout, stderr bytes.Buffer
	execParams := ExecParams{
		PodName:       rec.PodName,
		ContainerName: rec.ContainerName,
		Commands:      []string{"test", "-d", rec.Path},
		Stdout:        &stdout,
		Stderr:        &stderr,
	}
	return errors.Trace(c.Exec(ctx, execParams, cancel))
}
