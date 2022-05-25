// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload_test

import (
	"bytes"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/payload"
)

var _ = gc.Suite(&outputTabularSuite{})

type outputTabularSuite struct {
	testing.IsolationSuite
}

func (s *outputTabularSuite) TestFormatTabularOkay(c *gc.C) {
	pl := payload.NewPayload("spam", "a-application", 1, 0)
	pl.Labels = []string{"a-tag", "other"}
	formatted := payload.Formatted(pl)
	buff := &bytes.Buffer{}
	err := payload.FormatTabular(buff, formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(buff.String(), gc.Equals, `
[Unit Payloads]
Unit             Machine  Payload class  Status   Type    Id      Tags         
a-application/0  1        spam           running  docker  idspam  a-tag other  
`[1:])
}

func (s *outputTabularSuite) TestFormatTabularMinimal(c *gc.C) {
	pl := payload.NewPayload("spam", "a-application", 1, 0)
	formatted := payload.Formatted(pl)
	buff := &bytes.Buffer{}
	err := payload.FormatTabular(buff, formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(buff.String(), gc.Equals, `
[Unit Payloads]
Unit             Machine  Payload class  Status   Type    Id      Tags  
a-application/0  1        spam           running  docker  idspam        
`[1:])
}

func (s *outputTabularSuite) TestFormatTabularMulti(c *gc.C) {
	p10A := payload.NewPayload("spam", "a-application", 1, 0)
	p10A.Labels = []string{"a-tag"}
	p21A := payload.NewPayload("spam", "a-application", 2, 1)
	p21A.Status = "stopped"
	p21A.Labels = []string{"a-tag"}
	p21B := payload.NewPayload("spam", "a-application", 2, 1)
	p21B.ID += "B"
	p21x := payload.NewPayload("eggs", "a-application", 2, 1)
	p21x.Type = "kvm"
	p22A := payload.NewPayload("spam", "a-application", 2, 2)
	p10x := payload.NewPayload("ham", "another-application", 1, 0)
	p10x.Labels = []string{"other", "extra"}
	formatted := payload.Formatted(
		p10A,
		p21A,
		p21B,
		p21x,
		p22A,
		p10x,
	)
	buff := &bytes.Buffer{}
	err := payload.FormatTabular(buff, formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(buff.String(), gc.Equals, `
[Unit Payloads]
Unit                   Machine  Payload class  Status   Type    Id       Tags         
a-application/0        1        spam           running  docker  idspam   a-tag        
a-application/1        2        spam           stopped  docker  idspam   a-tag        
a-application/1        2        spam           running  docker  idspamB               
a-application/1        2        eggs           running  kvm     ideggs                
a-application/2        2        spam           running  docker  idspam                
another-application/0  1        ham            running  docker  idham    other extra  
`[1:])
}

func (s *outputTabularSuite) TestFormatTabularBadValue(c *gc.C) {
	bogus := "should have been []formattedPayload"
	err := payload.FormatTabular(nil, bogus)
	c.Check(err, gc.ErrorMatches, `expected value of type .*`)
}
