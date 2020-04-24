// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"os"
	"path/filepath"

	"github.com/juju/charm/v7"
	"github.com/juju/cmd"
	"github.com/juju/errors"
)

type componentHookFunction func() (Component, error)

func componentHookContext(ctx HookContext) componentHookFunction {
	return func() (Component, error) {
		compCtx, err := ContextComponent(ctx)
		if err != nil {
			// The component wasn't tracked properly.
			return nil, errors.Trace(err)
		}
		return compCtx, nil
	}
}

func readMetadata(ctx *cmd.Context) (*charm.Meta, error) {
	filename := filepath.Join(ctx.Dir, "metadata.yaml")
	file, err := os.Open(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer file.Close()
	meta, err := charm.ReadMeta(file)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return meta, nil
}
