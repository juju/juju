// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/deployment/charm"
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

var runSnapInfoCommand = func(ctx context.Context, packageName string) (string, error) {
	cmd := exec.CommandContext(ctx, "snap", "info", packageName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Annotatef(err, "snap info failed: %s", strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

var RunSnapInfoCommand = &runSnapInfoCommand

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

	vers, err := ParseSnapVersion(raw)
	if err != nil {
		return "", semversion.Zero, errors.Annotatef(err,
			"reading controller snap %q: cannot parse version %q", snapPath, raw)
	}
	return raw, vers, nil
}

func resolveSnapChannelVersion(ctx context.Context, channel string) (string, error) {
	out, err := runSnapInfoCommand(ctx, ControllerSnapPackageName)
	if err != nil {
		return "", errors.Trace(err)
	}

	pattern := fmt.Sprintf(`(?m)^\s*%s:\s*([^\s]+)`, regexp.QuoteMeta(channel))
	matches := regexp.MustCompile(pattern).FindStringSubmatch(out)
	if len(matches) < 2 {
		return "", errors.Errorf("unable to find controller snap version in channel %q", channel)
	}

	// validate the version of the snap matches following structure:
	//  4.1/edge:      4.1-beta2-cbd20b2
	//  4.0/stable:    4.0.5
	//  4.0/edge:      4.0.10-e0c5d0b
	//  3.6/beta:      3.6-beta2
	//
	// But not any of:
	//  4/beta:        ↑
	//  4.1/beta:      –
	v := strings.Split(matches[1], ".")
	if len(v) < 2 {
		return "", errors.Errorf("unable to resolve controller snap version in channel %q", channel)
	}

	return matches[1], nil
}

func resolveSnapChannel(channel charm.Channel) string {
	if !channel.Empty() {
		return channel.String()
	}

	return fmt.Sprintf(
		"%d.%d/edge", jujuversion.Current.Major, jujuversion.Current.Minor,
	)
}

func inspectLocalSnapVersion(ctx context.Context, path string) (semversion.Number, error) {
	_, vers, err := ReadSnapVersion(ctx, path)
	if err != nil {
		return semversion.Zero, errors.Annotatef(err,
			"inspecting local snap %q", path)
	}
	return vers, nil
}
