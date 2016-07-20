// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/testing/factory"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state/testing"
)

type clientAuthRootSuite struct {
	testing.StateSuite
}

var _ = gc.Suite(&clientAuthRootSuite{})

func (*clientAuthRootSuite) AssertCallGood(c *gc.C, client *clientAuthRoot, rootName string, version int, methodName string) {
	caller, err := client.FindMethod(rootName, version, methodName)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(caller, gc.NotNil)
}

func (*clientAuthRootSuite) AssertCallNotImplemented(c *gc.C, client *clientAuthRoot, rootName string, version int, methodName string) {
	caller, err := client.FindMethod(rootName, version, methodName)
	c.Check(errors.Cause(err), jc.Satisfies, isCallNotImplementedError)
	c.Assert(caller, gc.IsNil)
}

func (s *clientAuthRootSuite) AssertCallErrPerm(c *gc.C, client *clientAuthRoot, rootName string, version int, methodName string) {
	caller, err := client.FindMethod(rootName, version, methodName)
	c.Check(errors.Cause(err), gc.Equals, common.ErrPerm)
	c.Assert(caller, gc.IsNil)
}

func (s *clientAuthRootSuite) TestNormalUser(c *gc.C) {
	modelUser := s.Factory.MakeModelUser(c, nil)
	client := newClientAuthRoot(&fakeFinder{}, modelUser)
	s.AssertCallGood(c, client, "Application", 1, "Deploy")
	s.AssertCallGood(c, client, "UserManager", 1, "UserInfo")
	s.AssertCallNotImplemented(c, client, "Client", 1, "Unknown")
	s.AssertCallNotImplemented(c, client, "Unknown", 1, "Method")
}

func (s *clientAuthRootSuite) TestAdminUser(c *gc.C) {
	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{Access: description.WriteAccess})
	client := newClientAuthRoot(&fakeFinder{}, modelUser)
	s.AssertCallGood(c, client, "Client", 1, "FullStatus")
	s.AssertCallErrPerm(c, client, "ModelManager", 2, "ModifyModelAccess")
	s.AssertCallErrPerm(c, client, "ModelManager", 2, "CreateModel")
	s.AssertCallErrPerm(c, client, "Controller", 3, "DestroyController")

	modelUser = s.Factory.MakeModelUser(c, &factory.ModelUserParams{Access: description.AdminAccess})
	client = newClientAuthRoot(&fakeFinder{}, modelUser)
	s.AssertCallGood(c, client, "ModelManager", 2, "ModifyModelAccess")
	s.AssertCallGood(c, client, "ModelManager", 2, "CreateModel")
	s.AssertCallGood(c, client, "Controller", 3, "DestroyController")
}

func (s *clientAuthRootSuite) TestReadOnlyUser(c *gc.C) {
	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{Access: description.ReadAccess})
	client := newClientAuthRoot(&fakeFinder{}, modelUser)
	// deploys are bad
	s.AssertCallErrPerm(c, client, "Application", 1, "Deploy")
	// read only commands are fine
	s.AssertCallGood(c, client, "Client", 1, "FullStatus")
	// calls on the restricted root is also fine
	s.AssertCallGood(c, client, "UserManager", 1, "AddUser")
	s.AssertCallNotImplemented(c, client, "Client", 1, "Unknown")
	s.AssertCallNotImplemented(c, client, "Unknown", 1, "Method")
}

func isCallNotImplementedError(err error) bool {
	_, ok := err.(*rpcreflect.CallNotImplementedError)
	return ok
}

type fakeFinder struct{}

// FindMethod is the only thing we need to implement rpc.MethodFinder.
func (f *fakeFinder) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	_, _, err := lookupMethod(rootName, version, methodName)
	if err != nil {
		return nil, err
	}
	// Just return a valid caller.
	return &fakeCaller{}, nil
}

// fakeCaller implements a rpcreflect.MethodCaller. We don't care what the
// actual reflect.Types or values actually are, the caller just has to be
// valid.
type fakeCaller struct{}

func (*fakeCaller) ParamsType() reflect.Type {
	return reflect.TypeOf("")
}

func (*fakeCaller) ResultType() reflect.Type {
	return reflect.TypeOf("")
}

func (*fakeCaller) Call(_ /*objId*/ string, _ /*arg*/ reflect.Value) (reflect.Value, error) {
	return reflect.ValueOf(""), nil
}
