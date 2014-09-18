// +build !windows

package reboot

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

func writeScript(args []string, after int) (string, error) {
	tpl := `#!/bin/bash
sleep %d
%s`
	script := fmt.Sprintf(tpl, after, strings.Join(args, " "))

	f, err := tmpFile()
	if err != nil {
		return "", errors.Trace(err)
	}
	defer f.Close()

	_, err = f.WriteString(script)
	if err != nil {
		return "", errors.Trace(err)
	}
	name := f.Name()
	err = os.Chmod(name, 0755)
	if err != nil {
		return "", errors.Trace(err)
	}
	return name, nil
}

// scheduleAction will do a reboot or shutdown after given number of seconds
// this function executes the operating system's reboot binary with apropriate
// parameters to schedule the reboot
// If action is params.ShouldDoNothing, it will return immediately.
func scheduleAction(action params.RebootAction, after int) error {
	if action == params.ShouldDoNothing {
		return nil
	}
	args := []string{"shutdown"}
	switch action {
	case params.ShouldReboot:
		args = append(args, "-r")
	case params.ShouldShutdown:
		args = append(args, "-h")
	}
	args = append(args, "now")

	script, err := writeScript(args, after)
	if err != nil {
		return err
	}
	// Use the "at" command to schedule a reboot without blocking
	scheduled := []string{
		"at",
		"-f",
		script,
		"now",
	}
	return runCommand(scheduled)
}
