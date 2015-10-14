// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload/status"
)

var _ = gc.Suite(&outputTabularSuite{})

type outputTabularSuite struct{}

func (s *outputTabularSuite) TestFormatTabularOkay(c *gc.C) {
	payload := status.NewPayload("spam", "a-service", 1, 0)
	payload.Tags = []string{"a-tag", "other"}
	formatted := status.Formatted(payload)
	data, err := status.FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
[Unit Payloads]
UNIT             MACHINE PAYLOAD-CLASS STATUS  TYPE   ID     TAGS        
unit-a-service-0 1       spam          running docker idspam a-tag other 
`[1:])
}

func (s *outputTabularSuite) TestFormatTabularMinimal(c *gc.C) {
	payload := status.NewPayload("spam", "a-service", 1, 0)
	formatted := status.Formatted(payload)
	data, err := status.FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
[Unit Payloads]
UNIT             MACHINE PAYLOAD-CLASS STATUS  TYPE   ID     TAGS 
unit-a-service-0 1       spam          running docker idspam      
`[1:])
}

func (s *outputTabularSuite) TestFormatTabularMulti(c *gc.C) {
	p10A := status.NewPayload("spam", "a-service", 1, 0)
	p10A.Tags = []string{"a-tag"}
	p21A := status.NewPayload("spam", "a-service", 2, 1)
	p21A.Status = "stopped"
	p21A.Tags = []string{"a-tag"}
	p21B := status.NewPayload("spam", "a-service", 2, 1)
	p21B.ID += "B"
	p21x := status.NewPayload("eggs", "a-service", 2, 1)
	p21x.Type = "kvm"
	p22A := status.NewPayload("spam", "a-service", 2, 2)
	p10x := status.NewPayload("ham", "another-service", 1, 0)
	p10x.Tags = []string{"other", "extra"}
	formatted := status.Formatted(
		p10A,
		p21A,
		p21B,
		p21x,
		p22A,
		p10x,
	)
	data, err := status.FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
[Unit Payloads]
UNIT                   MACHINE PAYLOAD-CLASS STATUS  TYPE   ID      TAGS        
unit-a-service-0       1       spam          running docker idspam  a-tag       
unit-a-service-1       2       spam          stopped docker idspam  a-tag       
unit-a-service-1       2       spam          running docker idspamB             
unit-a-service-1       2       eggs          running kvm    ideggs              
unit-a-service-2       2       spam          running docker idspam              
unit-another-service-0 1       ham           running docker idham   other extra 
`[1:])
}

func (s *outputTabularSuite) TestFormatTabularBadValue(c *gc.C) {
	bogus := "should have been []formattedPayload"
	_, err := status.FormatTabular(bogus)

	c.Check(err, gc.ErrorMatches, `expected value of type .*`)
}
