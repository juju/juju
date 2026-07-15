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

	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/deployment/charm"
)

var runSnapInfoCommand = func(ctx context.Context, packageName string) (string, error) {
	cmd := exec.CommandContext(ctx, "snap", "info", packageName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Annotatef(err, "snap info failed: %s", strings.TrimSpace(string(out)))
	}
	return string(out), nil
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
