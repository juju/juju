// Copyright 2014 Cloudbase Solutions SRL
// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package uptime

import (
	"fmt"
)

//sys getTickCount64() (uptime uint64, err error) =  GetTickCount64

// Uptime returns the number of seconds since the system has booted
func Uptime() (int64, error) {
	uptime, err := getTickCount64()
	if err != nil {
		return 0, fmt.Errorf("Failed to get uptime. Error number: %v", err)
	}
	return int64(uptime) / 1000, nil
}
