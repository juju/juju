// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os/exec"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/internal/snapstore"
)

const (
	// unsquashfsRequiredMessage is returned to a native client that has no
	// unsquashfs helper available to read a controller snap's metadata. The
	// juju snap bundles squashfs-tools, so a confined client finds the helper
	// through its PATH; a native client must install it.
	unsquashfsRequiredMessage = "unsquashfs is required to inspect a controller snap; " +
		"install it with 'sudo apt install squashfs-tools', or run juju from the juju snap"
)

// findUnsquashfsFunc locates the unsquashfs helper. It is declared as a var
// for test injection, mirroring lookPathFunc in snap_build.go.
var findUnsquashfsFunc = func() (string, error) {
	return exec.LookPath("unsquashfs")
}

// runUnsquashfsCommand extracts meta/snap.yaml from a snap file via
// `unsquashfs -cat` and returns the file contents. Declared as a var for test
// injection.
var runUnsquashfsCommand = func(ctx context.Context, helper, snapPath string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, helper, "-cat", snapPath, "meta/snap.yaml")
	return cmd.CombinedOutput()
}

// RunUnsquashfsCommand and FindUnsquashfs expose the unsquashfs-backed reader's
// hooks for test injection from external packages.
var (
	RunUnsquashfsCommand = &runUnsquashfsCommand
	FindUnsquashfs       = &findUnsquashfsFunc
)

// ReadSnapVersion reads the raw `version:` value from a local snap file's
// meta/snap.yaml directly from its squashfs, without invoking the host `snap`
// binary. It returns the raw metadata string (for exact installed-version
// assertions) plus the normalised semversion.Number derived by ParseSnapVersion.
//
// The unsquashfs helper is resolved through PATH: the juju client snap stages
// squashfs-tools, and a native client must have unsquashfs installed. When the
// helper is unavailable an actionable error is returned so the caller can fail
// before provisioning.
func ReadSnapVersion(ctx context.Context, snapPath string) (string, semversion.Number, error) {
	helper, err := findUnsquashfsFunc()
	if err != nil {
		return "", semversion.Zero, errors.Annotate(err, unsquashfsRequiredMessage)
	}

	out, err := runUnsquashfsCommand(ctx, helper, snapPath)
	if err != nil {
		return "", semversion.Zero, errors.Annotatef(err,
			"reading controller snap %q: cannot extract meta/snap.yaml", snapPath)
	}

	var meta struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(out, &meta); err != nil {
		return "", semversion.Zero, errors.Annotatef(err,
			"reading controller snap %q: cannot parse meta/snap.yaml", snapPath)
	}

	raw := strings.TrimSpace(meta.Version)
	if raw == "" {
		return "", semversion.Zero, errors.Errorf(
			"reading controller snap %q: no version found in snap metadata", snapPath)
	}

	vers, err := snapstore.ParseSnapVersion(raw)
	if err != nil {
		return "", semversion.Zero, errors.Annotatef(err,
			"reading controller snap %q: cannot parse version %q", snapPath, raw)
	}
	return raw, vers, nil
}

func resolveSnapChannel(channel charm.Channel) string {
	if !channel.Empty() {
		return channel.String()
	}

	return "latest/edge"
}
