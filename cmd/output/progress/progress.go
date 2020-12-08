/*
 * Copyright (C) 2014-2015 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package progress

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

// Meter is an interface to show progress to the user
type Meter interface {
	io.Writer

	// Start progress with max "total" steps.
	Start(label string, total float64)

	// Set progress to the "current" step.
	Set(current float64)

	// SetTotal sets "total" steps needed.
	SetTotal(total float64)

	// Finished the progress display
	Finished()

	// Spin indicates indefinite activity by showing a spinner.
	Spin(msg string)

	// Notify the user of miscellaneous events
	Notify(string)
}

// MakeProgressBar creates an appropriate progress.Meter for the environ in
// which it is called:
//
// * if no terminal is attached, or we think we're running a test, a
//   minimalistic QuietMeter is returned.
// * otherwise, an ANSIMeter is returned.
//
func MakeProgressBar(stdout io.Writer) Meter {
	if terminal.IsTerminal(int(os.Stdin.Fd())) {
		return &ANSIMeter{
			terminal:    term{},
			stdout:      stdout,
			escapeChars: DefaultEscapeChars(),
		}
	}

	return QuietMeter{
		stdout: stdout,
	}
}

// NullMeter is a Meter that does nothing
type NullMeter struct{}

// Null is a default NullMeter instance
var Null = NullMeter{}

// Start progress with max "total" steps.
func (NullMeter) Start(string, float64) {}

// Set progress to the "current" step.
func (NullMeter) Set(float64) {}

// SetTotal sets "total" steps needed.
func (NullMeter) SetTotal(float64) {}

// Finished the progress display
func (NullMeter) Finished() {}

// Write the update to the meter.
func (NullMeter) Write(p []byte) (int, error) { return len(p), nil }

// Notify the user of miscellaneous events
func (NullMeter) Notify(string) {}

// Spin indicates indefinite activity by showing a spinner.
func (NullMeter) Spin(msg string) {}

// QuietMeter is a Meter that _just_ shows Notify()s.
type QuietMeter struct {
	NullMeter
	stdout io.Writer
}

// NewQuietMeter creates a new QuietMeter using the supplied stdout.
func NewQuietMeter(stdout io.Writer) *QuietMeter {
	return &QuietMeter{
		stdout: stdout,
	}
}

// Notify the user of miscellaneous events
func (m QuietMeter) Notify(msg string) {
	fmt.Fprintln(m.stdout, msg)
}
