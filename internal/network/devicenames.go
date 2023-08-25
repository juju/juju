// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"sort"
	"strconv"
	"unicode/utf8"
)

const (
	EOF = iota
	LITERAL
	NUMBER
)

type token int

type deviceNameScanner struct {
	src string

	// scanning state
	ch       rune // current character
	offset   int  // character offset
	rdOffset int  // reading offset (position of next ch)
}

type deviceName struct {
	name   string
	tokens []int
}

type devices []deviceName

func (d devices) Len() int {
	return len(d)
}

func (d devices) Less(i, j int) bool {
	if r := intCompare(d[i].tokens, d[j].tokens); r == -1 {
		return true
	} else {
		return false
	}
}

func (d devices) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}

// adapted from runtime/noasm.go
func intCompare(s1, s2 []int) int {
	l := len(s1)
	if len(s2) < l {
		l = len(s2)
	}
	if l == 0 || &s1[0] == &s2[0] {
		goto samebytes
	}
	for i := 0; i < l; i++ {
		c1, c2 := s1[i], s2[i]
		if c1 < c2 {
			return -1
		}
		if c1 > c2 {
			return +1
		}
	}
samebytes:
	if len(s1) < len(s2) {
		return -1
	}
	if len(s1) > len(s2) {
		return +1
	}
	return 0
}

func (s *deviceNameScanner) init(src string) {
	s.src = src
	s.ch = ' '
	s.offset = 0
	s.rdOffset = 0
	s.next()
}

func (s *deviceNameScanner) next() {
	if s.rdOffset < len(s.src) {
		s.offset = s.rdOffset
		r, w := rune(s.src[s.rdOffset]), 1
		s.rdOffset += w
		s.ch = r
	} else {
		s.offset = len(s.src)
		s.ch = -1 // EOF
	}
}

func (s *deviceNameScanner) peek() rune {
	if s.rdOffset < len(s.src) {
		r, _ := rune(s.src[s.rdOffset]), 1
		return r
	}
	return -1
}

func isDigit(ch rune) bool {
	return '0' <= ch && ch <= '9'
}

func (s *deviceNameScanner) scanNumber() string {
	// Treat leading zeros as discrete numbers as this aids the
	// natural sort ordering. We also only parse whole numbers;
	// floating point values are considered an integer- and
	// fractional-part.

	if s.ch == '0' && s.peek() == '0' {
		s.next()
		return "0"
	}

	cur := s.offset

	for isDigit(s.ch) {
		s.next()
	}

	return s.src[cur:s.offset]
}

func (s *deviceNameScanner) scan() (tok token, lit string) {
	switch ch := s.ch; {
	case -1 == ch:
		return EOF, ""
	case '0' <= ch && ch <= '9':
		return NUMBER, s.scanNumber()
	default:
		lit = string(s.ch)
		s.next()
		return LITERAL, lit
	}
}

func parseDeviceName(src string) deviceName {
	var s deviceNameScanner

	s.init(src)

	d := deviceName{name: src}

	for {
		tok, lit := s.scan()
		switch tok {
		case EOF:
			return d
		case LITERAL:
			x, _ := utf8.DecodeRuneInString(lit)
			d.tokens = append(d.tokens, int(x))
		case NUMBER:
			val, _ := strconv.Atoi(lit)
			d.tokens = append(d.tokens, val)
		}
	}
}

func parseDeviceNames(args ...string) devices {
	devices := make(devices, 0)

	for _, a := range args {
		devices = append(devices, parseDeviceName(a))
	}

	return devices
}

// NaturallySortDeviceNames returns an ordered list of names based on
// a natural ordering where 'natural' is an ordering of the string
// value in alphabetical order, execept that multi-digit numbers are
// ordered as a single character.
//
// For example, sorting:
//
//	[ br-eth10 br-eth1 br-eth2 ]
//
// would sort as:
//
//	[ br-eth1 br-eth2 br-eth10 ]
//
// In purely alphabetical sorting "br-eth10" would be sorted before
// "br-eth2" because "1" is sorted as smaller than "2", while in
// natural sorting "br-eth2" is sorted before "br-eth10" because "2"
// is sorted as smaller than "10".
//
// This also extends to multiply repeated numbers (e.g., VLANs).
//
// For example, sorting:
//
//	[ br-eth2 br-eth10.10 br-eth200.0 br-eth1.0 br-eth2.0 ]
//
// would sort as:
//
//	[ br-eth1.0 br-eth2 br-eth2.0 br-eth10.10 br-eth200.0 ]
func NaturallySortDeviceNames(names ...string) []string {
	if names == nil {
		return nil
	}

	devices := parseDeviceNames(names...)
	sort.Sort(devices)
	sortedNames := make([]string, len(devices))

	for i, v := range devices {
		sortedNames[i] = v.name
	}

	return sortedNames
}
