// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"
	"unicode"
)

func replaceSprintfVerbs(ev *evaluated) *evaluated {
	if ev == nil || ev.Content == "" {
		return ev
	}
	reps := findSprintfVerbs(ev.Content)
	if len(reps) == 0 {
		return ev
	}
	return applyReplacements(ev, reps)
}

func findSprintfVerbs(format string) []replacement {
	var reps []replacement
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		start := i
		i++
		if i >= len(format) {
			break
		}
		if format[i] == '%' {
			reps = append(reps, replacement{start: start, end: i + 1, token: "%"})
			continue
		}
		i = skipIndex(format, i)
		for i < len(format) && isSprintfFlag(format[i]) {
			i++
		}
		i = skipWidthPrec(format, i)
		i = skipIndex(format, i)
		if i >= len(format) {
			break
		}
		verb := format[i]
		if !isSprintfVerb(verb) {
			continue
		}
		end := i + 1
		reps = append(reps, replacement{
			start: start,
			end:   end,
			token: sprintfToken(verb),
		})
		i = end - 1
	}
	return reps
}

func skipIndex(s string, i int) int {
	if i >= len(s) || s[i] != '[' {
		return i
	}
	j := i + 1
	if j >= len(s) || !unicode.IsDigit(rune(s[j])) {
		return i
	}
	for j < len(s) && unicode.IsDigit(rune(s[j])) {
		j++
	}
	if j < len(s) && s[j] == ']' {
		return j + 1
	}
	return i
}

func skipWidthPrec(s string, i int) int {
	i = skipStarOrNum(s, i)
	if i < len(s) && s[i] == '.' {
		i++
		i = skipStarOrNum(s, i)
	}
	return i
}

func skipStarOrNum(s string, i int) int {
	if i >= len(s) {
		return i
	}
	if s[i] == '*' {
		return i + 1
	}
	for i < len(s) && unicode.IsDigit(rune(s[i])) {
		i++
	}
	return i
}

func isSprintfFlag(c byte) bool {
	switch c {
	case '#', '0', '+', '-', ' ':
		return true
	}
	return false
}

func isSprintfVerb(c byte) bool {
	return strings.ContainsRune("vTtbcdoqxXUeEfFgGspw", rune(c))
}

func sprintfToken(verb byte) string {
	switch verb {
	case 'd', 'b', 'o', 'x', 'X', 'e', 'E', 'f', 'F', 'g', 'G', 'c':
		return "1"
	default:
		return "t"
	}
}
