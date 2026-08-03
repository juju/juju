// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/juju/errors"

	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/tools"
)

const (
	snapcraftRequiredMessage = "snapcraft is required to build the controller snap; " +
		"install it with 'sudo snap install snapcraft --classic'"
	buildSnapProgressMessage = "Building controller snap via make build-snap; " +
		"this may take several minutes..."
)

var buildCommandFunc = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

var lookPathFunc = exec.LookPath

func snapArch(goarch string) string {
	switch goarch {
	case "arm":
		return "armhf"
	case "ppc64le":
		return "ppc64el"
	default:
		return goarch
	}
}

// BuildControllerSnap builds the controller snap from local source via 'make
// build-snap' and returns the path to the resulting .snap file. It is called
// from the bootstrap command's Run() when --build-snap is set, populating
// ControllerSnapPath so the existing upload pipeline handles the locally built
// snap.
var BuildControllerSnap = func(ctx context.Context) (string, error) {
	sourceRoot, err := tools.FindJujuSourceRoot()
	if err != nil {
		return "", errors.Annotate(err, "cannot locate juju source root")
	}

	if _, err := lookPathFunc("snapcraft"); err != nil {
		return "", errors.New(snapcraftRequiredMessage)
	}

	logger.Infof(ctx, buildSnapProgressMessage)

	out, err := buildCommandFunc(ctx, sourceRoot, "make", "build-snap")
	if err != nil {
		return "", fmt.Errorf(
			"building controller snap failed: %v\n%s\n"+
				"use --controller-snap-path to supply a pre-built snap instead",
			err, string(out),
		)
	}

	expectedFile := fmt.Sprintf(
		"jujud_%s_%s.snap",
		jujuversion.Current.String(),
		snapArch(runtime.GOARCH),
	)
	snapPath := filepath.Join(sourceRoot, expectedFile)

	if _, err := os.Stat(snapPath); err != nil {
		return "", fmt.Errorf(
			"controller snap build completed but expected file %q was not found at %s",
			expectedFile, sourceRoot,
		)
	}

	return snapPath, nil
}
