// +build !windows

package status

import "fmt"

// ClearScreen removes any character from the terminal
// using ANSI scape characters.
func ClearScreen() {
	fmt.Printf("\u001Bc")
}
