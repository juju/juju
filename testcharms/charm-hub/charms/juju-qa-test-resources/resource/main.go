// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Printf("%s-%s", runtime.GOARCH, runtime.GOOS)
}
