// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"fmt"

	gc "gopkg.in/check.v1"
)

type parserSuite struct{}

var _ = gc.Suite(&parserSuite{})

func (p *parserSuite) TestParser(c *gc.C) {
	//query := `((life=="alive" && status!="active") || life=="dead")`
	query := `life == "dead"`

	lex := NewLexer(query)
	parser := NewParser(lex)
	fmt.Println(parser.Run())
}
