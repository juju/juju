// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package status

import "fmt"

// ClearScreen removes any character from the terminal
// using ANSI scape characters.
func ClearScreen() {
	fmt.Printf("\u001Bc")
}
