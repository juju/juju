package testing_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jtesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&fakeAPISuite{})

type fakeAPISuite struct {
	testing.IsolationSuite
}

func (*fakeAPISuite) TestFakeAPI(c *gc.C) {
	var r root
	fakeUUID := "dead-beef"
	srv := apiservertesting.NewAPIServer(func(modelUUID string) interface{} {
		c.Check(modelUUID, gc.Equals, fakeUUID)
		return &r
	})
	defer srv.Close()
	info := &api.Info{
		Addrs:    srv.Addrs,
		CACert:   jtesting.CACert,
		ModelTag: names.NewModelTag("dead-beef"),
	}
	_, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(r.calledMethods, jc.DeepEquals, []string{"Login"})
}

type root struct {
	calledMethods []string
}

type facade struct {
	r *root
}

func (r *root) Admin(id string) (facade, error) {
	return facade{r}, nil
}

func (f facade) Login(req params.LoginRequest) (params.LoginResultV1, error) {
	f.r.calledMethods = append(f.r.calledMethods, "Login")
	return params.LoginResultV1{
		ModelTag:      names.NewModelTag("dead-beef").String(),
		ControllerTag: names.NewModelTag("dead-beef").String(),
		UserInfo: &params.AuthUserInfo{
			DisplayName: "foo",
			Identity:    "user-bar",
		},
		ServerVersion: "1.0.0",
	}, nil
}
