// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/modelconfig/service/testing"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/testing"
)

type providerServiceSuite struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) TestModelConfig(c *gc.C) {
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	st := testing.NewState()
	defer st.Close()

	cfg, err := config.New(config.NoDefaults, map[string]any{
		"name": "wallyworld",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	})
	c.Assert(err, jc.ErrorIsNil)

	for k, v := range cfg.AllAttrs() {
		st.Config[k] = fmt.Sprint(v)
	}

	svc := NewWatchableProviderService(st, st)

	cfg, err = svc.ModelConfig(ctx)
	c.Assert(err, jc.ErrorIsNil)

	// Note: `foo: bar` is not present because we don't take into account
	// the model defaults. If a SetModelConfig is called, the model defaults
	// will be taken into account.

	// Eventually this will sort it self out, but the initial read might
	// not match what the user expects.

	c.Check(cfg.AllAttrs(), jc.DeepEquals, map[string]any{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"secret-backend": "auto",
		"logging-config": "<root>=INFO",
	})
}
