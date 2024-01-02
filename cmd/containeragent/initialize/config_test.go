// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/containeragent/initialize"
)

func (s *initCommandSuit) TestConfigFromEnv(c *gc.C) {
	cfg := initialize.ConfigFromEnv{}
	c.Assert(cfg.Tag(), gc.DeepEquals, names.NewApplicationTag("gitlab"))
	c.Assert(cfg.CACert(), gc.DeepEquals, `ca-cert`)

	addrs, err := cfg.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.DeepEquals, []string{
		`1.1.1.1`, `2.2.2.2`,
	})

	apiInfo, ok := cfg.APIInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(apiInfo, gc.DeepEquals, &api.Info{
		Addrs: []string{
			`1.1.1.1`, `2.2.2.2`,
		},
		CACert:   `ca-cert`,
		ModelTag: names.NewModelTag("model1"),
		Tag:      names.NewApplicationTag("gitlab"),
		Password: `passwd`,
	})

}

func (s *initCommandSuit) TestDefaultIdentityOnK8S(c *gc.C) {
	ID, err := initialize.Identity()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ID.PodName, gc.DeepEquals, `gitlab-0`)
	c.Assert(ID.PodUUID, gc.DeepEquals, `gitlab-uuid`)
}
