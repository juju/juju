/*
 * Copyright (C) 2017 Canonical Ltd
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
	"time"
	"unicode"

	"golang.org/x/crypto/ssh/terminal"
)

// ANSI escape constants. These are the bits of the ANSI escapes (beyond \r)
// that we use (names of the terminfo capabilities, see terminfo(5))
const (
	// clear to end of line
	ClrEOL = "\033[K"
	// make cursor invisible
	CursorInvisible = "\033[?25l"
	// make cursor visible
	CursorVisible = "\033[?25h"
	// turn on reverse video
	EnterReverseMode = "\033[7m"
	// go back to normal video
	ExitAttributeMode = "\033[0m"
)

type Terminal interface {
	Width() int
}

var spinner = []string{"/", "-", "\\", "|"}

// ANSIMeter is a progress.Meter that uses ANSI escape codes to make
// better use of the available horizontal space.
type ANSIMeter struct {
	label   []rune
	total   float64
	written float64
	spin    int
	t0      time.Time

	terminal Terminal
	stdout   io.Writer
}

// NewANSIMeter creates a new ANSIMeter using the supplied stdout.
func NewANSIMeter(stdout io.Writer, term Terminal) *ANSIMeter {
	return &ANSIMeter{
		terminal: term,
		stdout:   stdout,
	}
}

// Start progress with max "total" steps.
func (p *ANSIMeter) Start(label string, total float64) {
	p.label = []rune(label)
	p.total = total
	p.t0 = time.Now().UTC()
	fmt.Fprint(p.stdout, CursorInvisible)
}

// SetTotal sets "total" steps needed.
func (p *ANSIMeter) SetTotal(total float64) {
	p.total = total
}

// Set progress to the "current" step.
func (p *ANSIMeter) Set(current float64) {
	if current < 0 {
		current = 0
	}
	if current > p.total {
		current = p.total
	}

	p.written = current
	col := p.terminal.Width()
	// time left: 5
	//    gutter: 1
	//     speed: 8
	//    gutter: 1
	//   percent: 4
	//    gutter: 1
	//          =====
	//           20
	// and we want to leave at least 10 for the label, so:
	//  * if      width <= 15, don't show any of this (progress bar is good enough)
	//  * if 15 < width <= 20, only show time left (time left + gutter = 6)
	//  * if 20 < width <= 29, also show percentage (percent + gutter = 5
	//  * if 29 < width      , also show speed (speed+gutter = 9)
	var percent, speed, timeleft string
	if col > 15 {
		since := time.Now().UTC().Sub(p.t0).Seconds()
		per := since / p.written
		left := (p.total - p.written) * per
		// XXX: duration unit string is controlled by translations, and
		// may carry a multibyte unit suffix
		timeleft = " " + FormatDuration(left)
		if col > 20 {
			percent = " " + p.Percent()
			if col > 29 {
				speed = " " + FormatBPS(p.written, since, -1)
			}
		}
	}

	rpercent := []rune(percent)
	rspeed := []rune(speed)
	rtimeleft := []rune(timeleft)
	msg := make([]rune, 0, col)
	// XXX: assuming terminal can display `col` number of runes
	msg = append(msg, Norm(col-len(rpercent)-len(rspeed)-len(rtimeleft), p.label)...)
	msg = append(msg, rpercent...)
	msg = append(msg, rspeed...)
	msg = append(msg, rtimeleft...)
	i := int(current * float64(col) / p.total)
	fmt.Fprint(p.stdout, "\r", EnterReverseMode, string(msg[:i]), ExitAttributeMode, string(msg[i:]))
}

// Spin indicates indefinite activity by showing a spinner.
func (p *ANSIMeter) Spin(msgstr string) {
	msg := []rune(msgstr)
	col := p.terminal.Width()
	if col-2 >= len(msg) {
		fmt.Fprint(p.stdout, "\r", string(Norm(col-2, msg)), " ", spinner[p.spin])
		p.spin++
		if p.spin >= len(spinner) {
			p.spin = 0
		}
	} else {
		fmt.Fprint(p.stdout, "\r", string(Norm(col, msg)))
	}
}

// Finished the progress display
func (p *ANSIMeter) Finished() {
	fmt.Fprint(p.stdout, "\r", ExitAttributeMode, CursorVisible, ClrEOL)
}

// Notify the user of miscellaneous events
func (p *ANSIMeter) Notify(msgstr string) {
	col := p.terminal.Width()
	fmt.Fprint(p.stdout, "\r", ExitAttributeMode, ClrEOL)

	msg := []rune(msgstr)
	var i int
	for len(msg) > col {
		for i = col; i >= 0; i-- {
			if unicode.IsSpace(msg[i]) {
				break
			}
		}
		if i < 1 {
			// didn't find anything; print the whole thing and try again
			fmt.Fprintln(p.stdout, string(msg[:col]))
			msg = msg[col:]
		} else {
			// found a space; print up to but not including it, and skip it
			fmt.Fprintln(p.stdout, string(msg[:i]))
			msg = msg[i+1:]
		}
	}
	fmt.Fprintln(p.stdout, string(msg))
}

func (p *ANSIMeter) Write(bs []byte) (n int, err error) {
	n = len(bs)
	p.Set(p.written + float64(n))

	return
}

func (p *ANSIMeter) SetWritten(written float64) {
	p.written = written
}

func (p *ANSIMeter) GetWritten() float64 {
	return p.written
}

func (p *ANSIMeter) Percent() string {
	if p.total == 0. {
		return "---%"
	}
	q := p.written * 100 / p.total
	if q > 999.4 || q < 0. {
		return "???%"
	}
	return fmt.Sprintf("%3.0f%%", q)
}

func Norm(col int, msg []rune) []rune {
	if col <= 0 {
		return []rune{}
	}
	out := make([]rune, col)
	copy(out, msg)
	d := col - len(msg)
	if d < 0 {
		out[col-1] = '…'
	} else {
		for i := len(msg); i < col; i++ {
			out[i] = ' '
		}
	}
	return out
}

type term struct{}

func (term) Width() int {
	col, _, _ := terminal.GetSize(0)
	if col <= 0 {
		// give up
		col = 80
	}
	return col
}
