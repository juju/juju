// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// filterable represents an object that can be passed through a filter.
type filterable interface {
	// matchAttr returns true if given attribute of the
	// object matches value. It returns an error if the
	// attribute is not recognised or the value is malformed.
	matchAttr(attr, value string) (bool, error)
}

type ec2filter []types.Filter

func (f ec2filter) ok(x filterable) (bool, error) {
next:
	for _, vs := range f {
		a := aws.ToString(vs.Name)
		for _, v := range vs.Values {
			if ok, err := x.matchAttr(a, v); ok {
				continue next
			} else if err != nil {
				return false, fmt.Errorf("bad attribute or value %q=%q for type %T: %v", a, v, x, err)
			}
		}
		return false, nil
	}
	return true, nil
}
