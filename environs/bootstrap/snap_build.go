// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
)

// buildCommandFunc runs the build command, streaming its output live to the
// supplied writers while also capturing it so callers can include it in an
// error message. Declared as a var for test injection.
var buildCommandFunc = func(
	ctx context.Context, stdout, stderr io.Writer, dir, name string, args ...string,
) ([]byte, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = io.MultiWriter(stdout, &buf)
	cmd.Stderr = io.MultiWriter(stderr, &buf)
	err := cmd.Run()
	return buf.Bytes(), err
}

var lookPathFunc = exec.LookPath

func snapArch(goarch string) string {
	switch goarch {
	case "ppc64le":
		return "ppc64el"
	default:
		return goarch
	}
}

// BuildControllerSnap builds the controller snap from local source via 'make
// jujud-snap-build' and returns the path to the resulting .snap file. It is called
// from the bootstrap command's Run() when --build-snap is set, populating
// ControllerSnapPath so the existing upload pipeline handles the locally built
// snap. Build output is streamed live to stdout and stderr.
//
// Declared as a var to allow test injection without a config struct; tests
// swap the value directly via the exported alias in export_test.go.
var BuildControllerSnap = func(ctx context.Context, stdout, stderr io.Writer) (string, error) {
	sourceRoot, err := tools.FindJujuSourceRoot()
	if err != nil {
		return "", errors.Annotate(err, "cannot locate juju source root")
	}

	if _, err := lookPathFunc("snapcraft"); err != nil {
		return "", errors.New(snapcraftRequiredMessage)
	}

	out, err := buildCommandFunc(ctx, stdout, stderr, sourceRoot, "make", "jujud-snap-build")
	if err != nil {
		return "", errors.Errorf(
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
	snapPath := filepath.Join(sourceRoot, "_build", "snap", expectedFile)

	if _, err := os.Stat(snapPath); err != nil {
		return "", errors.Errorf(
			"controller snap build completed but expected file %q was not found at %s: %v",
			expectedFile, sourceRoot, err,
		)
	}

	return snapPath, nil
}
