// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/core/actions"
)

func HandleAction(name string, params map[string]interface{}) (results map[string]interface{}, err error) {
	spec, ok := actions.PredefinedActionsSpec[name]
	if !ok {
		return nil, errors.Errorf("unexpected action %s", name)
	}
	if err := spec.ValidateParams(params); err != nil {
		return nil, errors.Errorf("invalid action parameters")
	}

	switch name {
	case actions.JujuRunActionName:
		return handleJujuRunAction(params)
	default:
		return nil, errors.Errorf("unexpected action %s", name)
	}
}

func handleJujuRunAction(params map[string]interface{}) (results map[string]interface{}, err error) {
	// The spec checks that the parameters are available so we don't need to check again here
	command, _ := params["command"].(string)

	// The timeout is passed in in nanoseconds(which are represented in go as int64)
	// But due to serialization it comes out as float64
	timeout, _ := params["timeout"].(float64)

	res, err := runCommandWithTimeout(command, time.Duration(timeout), clock.WallClock)

	actionResults := map[string]interface{}{}
	if res != nil {
		actionResults["Code"] = res.Code
		actionResults["Stdout"] = fmt.Sprintf("%s", res.Stdout)
		actionResults["Stderr"] = fmt.Sprintf("%s", res.Stderr)
	}
	return actionResults, err
}

func runCommandWithTimeout(command string, timeout time.Duration, clock clock.Clock) (*exec.ExecResponse, error) {
	cmd := exec.RunParams{
		Commands:    command,
		Environment: os.Environ(),
		Clock:       clock,
	}

	err := cmd.Run()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cancel chan struct{}
	if timeout != 0 {
		cancel = make(chan struct{})
		go func() {
			<-clock.After(timeout)
			close(cancel)
		}()
	}

	return cmd.WaitWithCancel(cancel)
}
