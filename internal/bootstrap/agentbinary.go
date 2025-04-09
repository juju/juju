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

	agenttools "github.com/juju/juju/agent/tools"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/logger"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/state/binarystorage"
)

const (
	// AgentCompressedBinaryName is the name of the agent binary.
	AgentCompressedBinaryName = "tools.tar.gz"
)

// AgentBinaryStorage is the interface that is used to store the agent binary.
type AgentBinaryStorage interface {
	// Add adds the agent binary to the storage.
	Add(context.Context, io.Reader, binarystorage.Metadata) error
}

// AgentBinaryStore is the service used to persist Juju agent binaries into the
// controller.
type AgentBinaryStore interface {
	AddAgentBinaryWithSHA256(context.Context, io.Reader, coreagentbinary.Version, int64, string) error
}

// PopulateAgentBinary is the function that is used to populate the agent
// binary at bootstrap.
func PopulateAgentBinary(
	ctx context.Context,
	dataDir string,
	storage AgentBinaryStorage,
	agentBinaryStore AgentBinaryStore,
	logger logger.Logger,
) (func(), error) {
	current := semversion.Binary{
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

	logger.Debugf(context.TODO(), "Adding agent binary: %v", agentTools.Version)

	if err := storage.Add(ctx, bytes.NewReader(data), metadata); err != nil {
		return nil, errors.Trace(err)
	}

	err = agentBinaryStore.AddAgentBinaryWithSHA256(
		ctx,
		bytes.NewReader(data),
		coreagentbinary.Version{
			Number: agentTools.Version.Number,
			Arch:   agentTools.Version.Arch,
		},
		agentTools.Size,
		agentTools.SHA256,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return func() {
		// Ensure that we remove the agent binary from disk.
		if err := os.Remove(binaryPath); err != nil {
			logger.Warningf(context.TODO(), "failed to remove agent binary: %v", err)
		}
		// Remove the sha that validates the agent binary file.
		shaFilePath := filepath.Join(rootPath, fmt.Sprintf("juju%s.sha256", current.String()))
		if err := os.Remove(shaFilePath); err != nil {
			logger.Warningf(context.TODO(), "failed to remove agent binary sha: %v", err)
		}
	}, nil
}
