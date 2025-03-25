// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

var _ Checker = (*BSONTotalSizeChecker)(nil)

// BSONTotalSizeChecker can be used to verify that the total bson-encoded size
// of one or more items does not exceed a particular limit.
type BSONTotalSizeChecker struct {
	maxSize int
	total   int
	lastErr error
}

// NewBSONTotalSizeChecker returns a BSONTotalSizeChecker instance with the
// specified maxSize limit. The maxSize parameter may also be set to zero to
// disable quota checks.
func NewBSONTotalSizeChecker(maxSize int) *BSONTotalSizeChecker {
	return &BSONTotalSizeChecker{
		maxSize: maxSize,
	}
}

// Check adds the serialized size of v to the current tally and updates the
// checker's error state.
func (c *BSONTotalSizeChecker) Check(v interface{}) {
	if c.lastErr != nil {
		return
	}

	size, err := effectiveSize(v)
	if err != nil {
		c.lastErr = err
		return
	} else if c.maxSize > 0 && c.total+size > c.maxSize {
		c.lastErr = errors.Errorf("max allowed size (%d) exceeded %w", c.maxSize, coreerrors.QuotaLimitExceeded)
	}

	c.total += size
}

// Outcome returns the check outcome or whether an error occurred within a call
// to the Check method.
func (c *BSONTotalSizeChecker) Outcome() error {
	return c.lastErr
}
