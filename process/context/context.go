// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

func init() {
	runner.RegisterComponentFunc("process", NewContext)
}

func NewContext() jujuc.ContextComponent {
	return &Context{}
}

type Context struct {
	processes map[string]process.Info
}

func (c *Context) Flush() error {
	return nil
}

func (c *Context) Get(id string, result interface{}) error {
	if _, ok := result.(*process.Info); !ok {
		return errors.Errorf("invalid type: expected process.Info, got %T", result)
	}

	pInfo, ok := c.processes[id]
	if !ok {
		return errors.NotFoundf("%s", id)
	}
	result = &pInfo
	return nil
}

func (c *Context) Set(id string, value interface{}) error {
	pInfo, ok := value.(*process.Info)
	if !ok {
		return errors.Errorf("invalid type: expected process.Info, got %T", value)
	}

	if id != pInfo.Name {
		return errors.Errorf("mismatch on name: %s != %s", id, pInfo.Name)
	}

	c.processes[id] = *pInfo
	return nil
}
