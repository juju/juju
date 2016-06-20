// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"fmt"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd/syslog/rfc5424"
)

type StructuredDataSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&StructuredDataSuite{})

func (s *StructuredDataSuite) TestStringOne(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "spam", "x=y"),
	}

	str := sd.String()

	c.Check(str, gc.Equals, `[spam x="y"]`)
}

func (s *StructuredDataSuite) TestStringMultiple(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "spam", "question=???"),
		newStubElement(stub, "eggs", "foo=bar"),
		newStubElement(stub, "spam", "answer=42"),
		newStubElement(stub, "ham", "foo=baz & bam"),
	}

	str := sd.String()

	c.Check(str, gc.Equals, `[spam question="???"][eggs foo="bar"][spam answer="42"][ham foo="baz & bam"]`)
}

func (s *StructuredDataSuite) TestStringZeroValue(c *gc.C) {
	var sd rfc5424.StructuredData

	str := sd.String()

	c.Check(str, gc.Equals, "-")
}

func (s *StructuredDataSuite) TestStringNoID(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "", "x=y"),
	}

	str := sd.String()

	c.Check(str, gc.Equals, `[ x="y"]`)
}

func (s *StructuredDataSuite) TestStringNoParams(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "spam"),
	}

	str := sd.String()

	c.Check(str, gc.Equals, `[spam]`)
}

func (s *StructuredDataSuite) TestStringMultipleParams(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "spam", "x=y", "w=z"),
	}

	str := sd.String()

	c.Check(str, gc.Equals, `[spam x="y" w="z"]`)
}

func (s *StructuredDataSuite) TestValidateOne(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "spam", "x=y"),
	}

	err := sd.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *StructuredDataSuite) TestValidateMultiple(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "spam", "question=???"),
		newStubElement(stub, "eggs", "foo=bar"),
		newStubElement(stub, "spam", "answer=42"),
		newStubElement(stub, "ham", "foo=baz & bam"),
	}

	err := sd.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *StructuredDataSuite) TestValidateZeroValue(c *gc.C) {
	var sd rfc5424.StructuredData

	err := sd.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *StructuredDataSuite) TestValidateBadElementInvalid(c *gc.C) {
	stub := &testing.Stub{}
	failure := fmt.Errorf("<invalid>")
	stub.SetErrors(failure)
	sd := rfc5424.StructuredData{
		newStubElement(stub, "spam", "x=y"),
	}

	err := sd.Validate()

	c.Check(err, gc.ErrorMatches, `element 0 not valid: <invalid>`)
}

func (s *StructuredDataSuite) TestValidateBadElementEmptyID(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "", "x=y"),
	}

	err := sd.Validate()

	c.Check(err, gc.ErrorMatches, `element 0 not valid: empty ID`)
}

func (s *StructuredDataSuite) TestValidateBadElementBadID(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "spam", "x=y"),
		newStubElement(stub, "=eggs", "foo=bar"),
	}

	err := sd.Validate()

	c.Check(err, gc.ErrorMatches, `element 1 not valid: invalid ID "=eggs": invalid character`)
}

func (s *StructuredDataSuite) TestValidateBadElementBadParam(c *gc.C) {
	stub := &testing.Stub{}
	sd := rfc5424.StructuredData{
		newStubElement(stub, "spam", "foo=", "=y"),
	}

	err := sd.Validate()

	c.Check(err, gc.ErrorMatches, `element 0 not valid: param 1 not valid: empty Name`)
}

type stubElement struct {
	stub *testing.Stub

	ReturnID     rfc5424.StructuredDataName
	ReturnParams []rfc5424.StructuredDataParam
}

func newStubElement(stub *testing.Stub, id string, paramStrs ...string) *stubElement {
	params := make([]rfc5424.StructuredDataParam, len(paramStrs))
	for i, str := range paramStrs {
		parts := strings.SplitN(str, "=", 2)
		params[i].Name = rfc5424.StructuredDataName(parts[0])
		params[i].Value = rfc5424.StructuredDataParamValue(parts[1])
	}

	return &stubElement{
		stub:         stub,
		ReturnID:     rfc5424.StructuredDataName(id),
		ReturnParams: params,
	}
}
func (s *stubElement) ID() rfc5424.StructuredDataName {
	s.stub.AddCall("ID")
	s.stub.NextErr() // pop one off

	return s.ReturnID
}

func (s *stubElement) Params() []rfc5424.StructuredDataParam {
	s.stub.AddCall("Params")
	s.stub.NextErr() // pop one off

	return s.ReturnParams
}

func (s *stubElement) Validate() error {
	s.stub.AddCall("Validate")
	if err := s.stub.NextErr(); err != nil {
		return err
	}

	return nil
}
