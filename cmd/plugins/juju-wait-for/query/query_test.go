// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type querySuite struct{}

var _ = gc.Suite(&querySuite{})

func (s *querySuite) TestQuery(c *gc.C) {
	src := `life == death || life == life`

	query, err := Parse(src)
	c.Assert(err, jc.ErrorIsNil)
	fmt.Println(query.Run(GlobalScope{}))
}

type GlobalScope struct{}

func (g GlobalScope) GetIdentValue(name string) (interface{}, error) {
	return name, nil
}
