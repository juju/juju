// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package initialize_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/k8sagent/initialize"
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
	ID, err := initialize.DefaultIdentity()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ID.PodName, gc.DeepEquals, `gitlab-0`)
	c.Assert(ID.PodUUID, gc.DeepEquals, `gitlab-uuid`)
}

func (s *initCommandSuit) TestDefaultIdentityOnECS(c *gc.C) {
	mux := http.NewServeMux()
	mux.HandleFunc("/task", func(w http.ResponseWriter, req *http.Request) {
		_, err := fmt.Fprintf(w, `
                        {
                            "Cluster": "sagittarius",
                            "TaskARN": "arn:aws:ecs:us-west-2:111122223333:task/default/d3adb33f",
                            "Family": "nginx"
                        }
                `)
		c.Assert(err, jc.ErrorIsNil)
	})
	srv := httptest.NewServer(mux)

	c.Assert(os.Setenv("ECS_CONTAINER_METADATA_URI_V4", srv.URL), jc.ErrorIsNil)
	defer func() {
		srv.Close()
		c.Assert(os.Setenv("ECS_CONTAINER_METADATA_URI_V4", ""), jc.ErrorIsNil)
	}()

	ID, err := initialize.DefaultIdentity()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ID.PodName, gc.DeepEquals, `arn:aws:ecs:us-west-2:111122223333:task/default/d3adb33f`)
	c.Assert(ID.PodUUID, gc.DeepEquals, `d3adb33f`)
}
