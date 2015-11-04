// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"os"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/testing"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

type BaseCrossModelSuite struct {
	jujutesting.BaseSuite
}

func (s *BaseCrossModelSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

func (s *BaseCrossModelSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	memstore := configstore.NewMem()
	s.PatchValue(&configstore.Default, func() (configstore.Storage, error) {
		return memstore, nil
	})
	os.Setenv(osenv.JujuEnvEnvKey, "testing")
	info := memstore.CreateInfo("testing")
	info.SetBootstrapConfig(map[string]interface{}{"random": "extra data"})
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   []string{"127.0.0.1:12345"},
		Hostnames:   []string{"localhost:12345"},
		CACert:      jujutesting.CACert,
		EnvironUUID: "env-uuid",
	})
	info.SetAPICredentials(configstore.APICredentials{
		User:     "user-test",
		Password: "password",
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
}
