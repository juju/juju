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
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/state/binarystorage"
	jujuversion "github.com/juju/juju/version"
)

// Logger represents the logging methods called.
type Logger interface {
	Warningf(message string, args ...any)
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
func PopulateAgentBinary(ctx context.Context, dataDir string, storage AgentBinaryStorage, logger Logger) (func(), error) {
	current := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}

	agentTools, err := agenttools.ReadTools(dataDir, current)
	if err != nil {
		return nil, fmt.Errorf("cannot read agent binary: %w", err)
	}

	rootPath := agenttools.SharedToolsDir(dataDir, current)
	binaryPath := filepath.Join(rootPath, AgentCompressedBinaryName)

	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	metadata := binarystorage.Metadata{
		Version: agentTools.Version.String(),
		Size:    agentTools.Size,
		SHA256:  agentTools.SHA256,
	}

	logger.Debugf("Adding agent binary: %v", agentTools.Version)

	// If the hash already exists, we don't need to add it again.
	if err := storage.Add(ctx, bytes.NewReader(data), metadata); err != nil && !errors.Is(err, objectstoreerrors.ErrHashAlreadyExists) {
		return nil, errors.Trace(err)
	}

	return func() {
		// Ensure that we remove the agent binary from disk.
		if err := os.Remove(binaryPath); err != nil {
			logger.Warningf("failed to remove agent binary: %v", err)
		}
		// Remove the sha that validates the agent binary file.
		shaFilePath := filepath.Join(rootPath, fmt.Sprintf("juju%s.sha256", current.String()))
		if err := os.Remove(shaFilePath); err != nil {
			logger.Warningf("failed to remove agent binary sha: %v", err)
		}
	}, nil
}
