// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/state/binarystorage"
	jujuversion "github.com/juju/juju/version"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
}

const (
	// AgentCompressedBinaryName is the name of the agent binary.
	AgentCompressedBinaryName = "tools.tar.gz"
)

// AgentBinaryStorage is the interface that is used to store the agent binary.
type AgentBinaryStorage interface {
	// Add adds the agent binary to the storage.
	Add(context.Context, io.Reader, binarystorage.Metadata) error
}

// PopulateAgentBinary is the function that is used to populate the agent
// binary at bootstrap.
func PopulateAgentBinary(ctx context.Context, dataDir string, storage AgentBinaryStorage, cfg controller.Config, logger Logger) error {
	current := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}

	agentTools, err := agenttools.ReadTools(dataDir, current)
	if err != nil {
		return fmt.Errorf("cannot read tools: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(
		agenttools.SharedToolsDir(dataDir, current),
		AgentCompressedBinaryName,
	))
	if err != nil {
		return errors.Trace(err)
	}

	metadata := binarystorage.Metadata{
		Version: agentTools.Version.String(),
		Size:    agentTools.Size,
		SHA256:  agentTools.SHA256,
	}

	logger.Debugf("Adding agent binary: %v", agentTools.Version)
	if err := storage.Add(ctx, bytes.NewReader(data), metadata); err != nil {
		return errors.Trace(err)
	}

	return nil
}
