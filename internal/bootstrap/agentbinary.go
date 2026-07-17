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
	"strings"

	"github.com/juju/errors"

	agenttools "github.com/juju/juju/agent/tools"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/logger"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	coretools "github.com/juju/juju/internal/tools"
)

const (
	// AgentCompressedBinaryName is the name of the agent binary.
	AgentCompressedBinaryName = "tools.tar.gz"
)

// AgentBinaryStore is the service used to persist Juju agent binaries into the
// controller.
type AgentBinaryStore interface {
	// AddAgentBinaryWithSHA256 adds a new agent binary to the object store and
	// saves its metadata to the database. The following errors can be returned:
	// - [github.com/juju/juju/core/errors.NotSupported] if the architecture is
	// not supported.
	// - [github.com/juju/juju/domain/agentbinary/errors.AlreadyExists] if an
	// agent binary already exists for this version and architecture.
	// - [github.com/juju/juju/domain/agentbinary/errors.ObjectNotFound] if
	// there was a problem referencing the
	// agent binary metadata with the previously saved binary object. This error
	// should be considered an internal problem. It is
	// discussed here to make the caller aware of future problems.
	// - [github.com/juju/juju/core/errors.NotValid] when the agent version is
	// not considered valid.
	// - [github.com/juju/juju/domain/agentbinary/errors.HashMismatch] when the
	// expected sha does not match that which was computed against the binary
	// data.
	AddAgentBinaryWithSHA256(context.Context, io.Reader, coreagentbinary.Version, int64, string) error
}

// PopulateAgentBinary is the function that is used to populate the agent
// binary at bootstrap.
func PopulateAgentBinary(
	ctx context.Context,
	dataDir string,
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
		// Under snap confinement HostOSTypeName is the base (ubuntu-core ->
		// genericlinux) while cloud-init stages agent binaries with the host
		// series (ubuntu). Fall back to any on-disk tools matching version+arch.
		alt, altErr := findAgentToolsByNumberArch(dataDir, current.Number, current.Arch)
		if altErr != nil {
			return nil, fmt.Errorf("cannot read agent binary: %w (also: %v)", err, altErr)
		}
		agentTools = alt
		current = agentTools.Version
	}

	rootPath := agenttools.SharedToolsDir(dataDir, current)
	binaryPath := filepath.Join(rootPath, AgentCompressedBinaryName)
	shaFilePath := filepath.Join(rootPath, fmt.Sprintf("juju%s.sha256", agentTools.Version.String()))

	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Debugf(ctx, "Adding agent binary: %v", agentTools.Version)

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
	if err != nil && !errors.Is(err, agentbinaryerrors.AlreadyExists) {
		return nil, errors.Trace(err)
	}

	return func() {
		// Ensure that we remove the agent binary from disk.
		if err := os.Remove(binaryPath); err != nil {
			logger.Warningf(ctx, "failed to remove agent binary: %v", err)
		}
		// Remove the sha that validates the agent binary file.
		if err := os.Remove(shaFilePath); err != nil {
			logger.Warningf(ctx, "failed to remove agent binary sha: %v", err)
		}
	}, nil
}

// findAgentToolsByNumberArch finds tools under dataDir/tools whose binary
// series version matches number+arch, ignoring the OS release component.
func findAgentToolsByNumberArch(dataDir string, number semversion.Number, hostArch string) (*coretools.Tools, error) {
	entries, err := os.ReadDir(filepath.Join(dataDir, "tools"))
	if err != nil {
		return nil, errors.Trace(err)
	}
	wantPrefix := number.String() + "-"
	wantSuffix := "-" + hostArch
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, wantPrefix) || !strings.HasSuffix(name, wantSuffix) {
			continue
		}
		vers, err := semversion.ParseBinary(name)
		if err != nil {
			continue
		}
		tools, err := agenttools.ReadTools(dataDir, vers)
		if err != nil {
			continue
		}
		return tools, nil
	}
	return nil, errors.NotFoundf("agent tools matching %s-%s under %s/tools", number, hostArch, dataDir)
}
