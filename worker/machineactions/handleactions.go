// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"encoding/base64"
	"os"
	"time"
	"unicode/utf8"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v3/exec"

	"github.com/juju/juju/core/actions"
)

// RunAsUser is the user that the machine juju-exec action is executed as.
var RunAsUser = "ubuntu"

// HandleAction receives a name and a map of parameters for a given machine action.
// It will handle that action in a specific way and return a results map suitable for ActionFinish.
func HandleAction(name string, params map[string]interface{}) (results map[string]interface{}, err error) {
	spec, ok := actions.PredefinedActionsSpec[name]
	if !ok {
		return nil, errors.Errorf("unexpected action %s", name)
	}
	if err := spec.ValidateParams(params); err != nil {
		return nil, errors.Errorf("invalid action parameters")
	}

	if actions.IsJujuExecAction(name) {
		return handleJujuExecAction(params)
	} else {
		return nil, errors.Errorf("unexpected action %s", name)
	}
}

func handleJujuExecAction(params map[string]interface{}) (results map[string]interface{}, err error) {
	// The spec checks that the parameters are available so we don't need to check again here
	command, _ := params["command"].(string)
	logger.Tracef("juju run %q", command)

	// The timeout is passed in in nanoseconds(which are represented in go as int64)
	// But due to serialization it comes out as float64
	timeout, _ := params["timeout"].(float64)

	res, err := runCommandWithTimeout(command, time.Duration(timeout), clock.WallClock)
	if err != nil {
		return nil, errors.Trace(err)
	}

	actionResults := map[string]interface{}{}
	actionResults["return-code"] = res.Code
	storeOutput(actionResults, "stdout", res.Stdout)
	storeOutput(actionResults, "stderr", res.Stderr)

	return actionResults, nil
}

func runCommandWithTimeout(command string, timeout time.Duration, clock clock.Clock) (*exec.ExecResponse, error) {
	cmd := exec.RunParams{
		Commands:    command,
		Environment: os.Environ(),
		Clock:       clock,
		User:        RunAsUser,
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

func encodeBytes(input []byte) (value string, encoding string) {
	if utf8.Valid(input) {
		value = string(input)
		encoding = "utf8"
	} else {
		value = base64.StdEncoding.EncodeToString(input)
		encoding = "base64"
	}
	return value, encoding
}

func storeOutput(values map[string]interface{}, key string, input []byte) {
	value, encoding := encodeBytes(input)
	values[key] = value
	if encoding != "utf8" {
		values[key+"Encoding"] = encoding
	}
}
