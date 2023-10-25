// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import "fmt"

// APILostDuringUpgrade is returned when the API connection is lost
// during an upgrade.
type APILostDuringUpgrade struct {
	err error
}

// NewAPILostDuringUpgrade returns a new APILostDuringUpgrade error.
func NewAPILostDuringUpgrade(err error) *APILostDuringUpgrade {
	return &APILostDuringUpgrade{
		err: err,
	}
}

func (e *APILostDuringUpgrade) Is(err error) bool {
	_, ok := err.(*APILostDuringUpgrade)
	return ok
}

func (e *APILostDuringUpgrade) Error() string {
	return fmt.Sprintf("API connection lost during upgrade: %v", e.err)
}
