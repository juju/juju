package serveradmin_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/serveradmin"
	"github.com/juju/juju/apiserver/params"
)

type serveradminSuite struct {
	baseSuite
}

var _ = gc.Suite(&serveradminSuite{})

func (s *serveradminSuite) TestClient(c *gc.C) {
	facade := serveradmin.ExposeFacade(s.client)

	c.Check(facade.Name(), gc.Equals, "ServerAdmin")
}

func (s *serveradminSuite) TestIdentityProvider(c *gc.C) {
	cleanup := serveradmin.PatchClientFacadeCall(s.client, func(request string, args interface{}, response interface{}) error {
		c.Assert(request, gc.Equals, "IdentityProvider")
		c.Assert(args, gc.IsNil)
		if result, ok := response.(*params.IdentityProviderResult); ok {
			result.IdentityProvider = &params.IdentityProviderInfo{
				PublicKey: "foo",
				Location:  "bar",
			}
			return nil
		}
		return fmt.Errorf("wrong result type: %T", response)
	})
	defer cleanup()
	info, err := s.client.IdentityProvider()
	c.Assert(err, gc.IsNil)
	c.Assert(info.PublicKey, gc.Equals, "foo")
	c.Assert(info.Location, gc.Equals, "bar")
}

func (s *serveradminSuite) TestSetIdentityProvider(c *gc.C) {
	cleanup := serveradmin.PatchClientFacadeCall(s.client, func(request string, args interface{}, response interface{}) error {
		c.Assert(request, gc.Equals, "SetIdentityProvider")
		setArgs, ok := args.(params.SetIdentityProvider)
		c.Assert(ok, jc.IsTrue, gc.Commentf("unexpected type: %T", args))
		c.Assert(setArgs.IdentityProvider.PublicKey, gc.Equals, "foo")
		c.Assert(setArgs.IdentityProvider.Location, gc.Equals, "bar")
		return nil
	})
	defer cleanup()
	err := s.client.SetIdentityProvider("foo", "bar")
	c.Assert(err, gc.IsNil)
}
