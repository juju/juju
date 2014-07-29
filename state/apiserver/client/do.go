// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
)

// Actions returns the available actions
func (c *Client) Do(args params.Do) (params.DoResult, error) {
	result, err := params.DoResult{}, error(nil)

	unit, err := c.api.state.Unit(args.UnitName)
	if err != nil {
		return result, err
	}

	action, err := unit.AddAction(args.Action, nil)
	if err != nil {
		return result, err
	}
	result.Id = action.Id()
	result.Status = "running"

	if args.Async {
		return result, nil
	}

	// TODO(jcw4) should use common UUID, avoid this conversion from
	// action id to action result id
	arId := c.api.state.ActionResultIdFromActionId(result.Id)

	watcher := unit.WatchActionResults()
	timeout := time.After(5 * time.Minute)
	for {
		select {
		case changes, ok := <-watcher.Changes():
			if !ok {
				return result, errors.Errorf("unknown error waiting for action to complete")
			}
			for _, id := range changes {
				if id == arId {
					result.Complete = true
					ar, err := c.api.state.ActionResult(id)
					if err != nil {
						return result, err
					}

					result.Output = ar.Output()
					switch ar.Status() {
					case state.ActionCompleted:
						result.Status = "succeeded"
					case state.ActionFailed:
						result.Status = "failed"
					default:
						result.Status = "unknown"
					}

					return result, nil
				}
			}
		case <-timeout:
			return result, errors.Errorf("timed out waiting for action result")
		}
	}
}
