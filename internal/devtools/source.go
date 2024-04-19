// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package devtools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

var SourceDir = sourceDir

func sourceDir() (string, error) {
	_, b, _, _ := runtime.Caller(0)
	modCmd := exec.Command("go", "list", "-m", "-json")
	modCmd.Dir = filepath.Dir(b)
	modInfo, err := modCmd.Output()
	if err != nil {
		return "", fmt.Errorf("requires juju binary to be built locally: %w", err)
	}
	mod := struct {
		Path string `json:"Path"`
		Dir  string `json:"Dir"`
	}{}
	err = json.Unmarshal(modInfo, &mod)
	if err != nil {
		return "", fmt.Errorf("requires juju binary to be built locally: %w", err)
	}
	if mod.Path != "github.com/juju/juju" {
		return "", fmt.Errorf("cannot use juju binary built for --dev")
	}
	return mod.Dir, nil
}
