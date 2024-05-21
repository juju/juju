// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"bufio"
	"io"
	"strings"
	"unicode"

	"github.com/juju/errors"

	"gopkg.in/gobwas/glob.v0"
)

var (
	ignorePatternReplacer = strings.NewReplacer(
		"\\#", "#",
		"\\!", "!",
		"\\ ", " ",
	)
)

type ruleResult uint8

const (
	// ruleResultKeep indicates that a file did not match an ignore
	// rule and should be copied.
	ruleResultKeep ruleResult = iota

	// ruleResultSkip indicates that a file matched an ignore rule and
	// should not be copied.
	ruleResultSkip

	// ruleResultKeep indicates that a file matched an inverted ignore rule
	// and should be copied.
	ruleResultForceKeep
)

type ignoreRuleEvalFn func(path string, isDir bool) ruleResult

// ignoreOnlyDirs constructs a ignoreRuleEvalFn that always returns
// ruleResultKeep for input paths that are not directories or the result of
// evaluating r for directory paths.
func ignoreOnlyDirs(r ignoreRuleEvalFn) ignoreRuleEvalFn {
	return func(path string, isDir bool) ruleResult {
		if !isDir {
			return ruleResultKeep
		}

		return r(path, isDir)
	}
}

// negateIgnoreRule constructs a ignoreRuleEvalFn that returns
// ruleResultForceKeep if r evaluates to ruleResultSkip. This function enables
// the construction of negateed ignore rules that force-include a file even
// though it was previously excluded by another rule.
func negateIgnoreRule(r ignoreRuleEvalFn) ignoreRuleEvalFn {
	return func(path string, isDir bool) ruleResult {
		if res := r(path, isDir); res == ruleResultSkip {
			return ruleResultForceKeep
		}

		return ruleResultKeep
	}
}

// ignoreGlobMatch constructs a ignoreRuleEvalFn that returns ruleResultSkip
// when the input matches any of the provided glob patterns. If an invalid glob
// pattern is provided then ignoreGlobMatch returns an error.
func ignoreGlobMatch(pattern string) (ignoreRuleEvalFn, error) {
	var (
		err              error
		expandedPatterns = genIgnorePatternPermutations(pattern)
		globPats         = make([]glob.Glob, len(expandedPatterns))
	)

	for i, pat := range expandedPatterns {
		globPats[i], err = glob.Compile(pat, '/')
		if err != nil {
			return nil, err
		}
	}

	return func(path string, isDir bool) ruleResult {
		for _, globPat := range globPats {
			if globPat.Match(path) {
				return ruleResultSkip
			}
		}

		return ruleResultKeep
	}, nil
}

type ignoreRuleset []ignoreRuleEvalFn

// newIgnoreRuleset reads the contents of a .jujuignore file from r and returns
// back an ignoreRuleset that can be used to match files against the set of
// exclusion rules.
//
// .jujuignore files use the same syntax as .gitignore files. For more details
// see: https://git-scm.com/docs/gitignore#_pattern_format
func newIgnoreRuleset(r io.Reader) (ignoreRuleset, error) {
	var (
		lineNo int
		rs     ignoreRuleset
		s      = bufio.NewScanner(r)
	)

	for s.Scan() {
		lineNo++

		// Cleanup leading whitespace; ignore empty and comment lines
		rule := strings.TrimLeftFunc(s.Text(), unicode.IsSpace)
		if len(rule) == 0 || rule[0] == '#' {
			continue
		}

		r, err := compileIgnoreRule(rule)
		if err != nil {
			return nil, errors.Annotatef(err, "[line %d]", lineNo)
		}

		rs = append(rs, r)
	}

	if err := s.Err(); err != nil {
		return nil, err
	}

	return rs, nil
}

// Match returns true if path matches any of the ignore rules in the set.
func (rs ignoreRuleset) Match(path string, isDir bool) bool {
	// To properly support start-of-pathname patterns all paths must
	// begin with a /
	if len(path) > 0 && path[0] != '/' {
		path = "/" + path
	}

	var keep = true
	for _, r := range rs {
		switch r(path, isDir) {
		case ruleResultKeep:
			// Keep file unless already excluded
			if !keep {
				continue
			}

			keep = true
		case ruleResultSkip:
			keep = false
		case ruleResultForceKeep:
			// Keep file even if already excluded (inverted rule)
			keep = true
		}
	}

	return !keep
}

// compileIgnoreRule returns an ignoreRuleEvalFn for the provided rule.
func compileIgnoreRule(rule string) (ignoreRuleEvalFn, error) {
	var (
		negateRule     bool
		applyToDirOnly bool
	)

	// If the rule begins with a '!' then the pattern is negated; any
	// matching file excluded by a previous pattern will become included
	// again.
	if strings.HasPrefix(rule, "!") {
		rule = strings.TrimPrefix(rule, "!")
		negateRule = true
	}

	rule = unescapeIgnorePattern(rule)

	// If the rule ends in a '/' then the slash is stripped off but the
	// rule will only apply to directories.
	if strings.HasSuffix(rule, "/") {
		rule = strings.TrimSuffix(rule, "/")
		applyToDirOnly = true
	}

	// A leading "**" followed by a slash means match in all directories.
	// "**/foo" is equivalent to "foo/bar" so we can actually trim it.
	rule = strings.TrimPrefix(rule, "**/")

	// A leading slash matches the beginning of the pathname. For example,
	// "/*.go" matches "foo.go" but not "bar/foo.go". In all other cases
	// the pattern applies at any location (substring pattern) and we need
	// to prefix it with "**/" (** behaves like * but also matches path
	// separators)
	if !strings.HasPrefix(rule, "/") {
		rule = "**/" + rule
	}

	fn, err := ignoreGlobMatch(rule)
	if err != nil {
		return nil, err
	}

	if applyToDirOnly {
		fn = ignoreOnlyDirs(fn)
	}

	if negateRule {
		fn = negateIgnoreRule(fn)
	}

	return fn, nil
}

// unescapeIgnorePattern removes unescaped trailing spaces and unescapes spaces,
// hashes and bang characters in pattern.
func unescapeIgnorePattern(pattern string) string {
	// Trim trailing spaces, unless they are escaped with a backslash
	for index := len(pattern) - 1; index > 0 && pattern[index] == ' '; index-- {
		if pattern[index-1] != '\\' {
			pattern = pattern[:index]
		}
	}

	// Unescape supported characters
	return ignorePatternReplacer.Replace(pattern)
}

// genIgnorePatternPermutations receives as input a string possibly containing
// one or more double-star separator patterns (/**/) and generates a list of
// additional glob patterns that allow matching zero-or-more items at the
// location of each double-star separator.
//
// For example, given "foo/**/bar/**/baz" as input, this function returns:
//   - foo/**/bar/**/baz
//   - foo/bar/**/baz
//   - foo/**/bar/baz
//   - foo/bar/baz
func genIgnorePatternPermutations(in string) []string {
	var (
		out       []string
		remaining = []string{in}

		addedPatternWithoutStars bool
	)

	for len(remaining) != 0 {
		next := remaining[0]
		remaining = remaining[1:]

		// Split on the double-star separator; stop if no the pattern
		// does not contain any more double-star separators.
		parts := strings.Split(next, "/**/")
		if len(parts) == 1 {
			if !addedPatternWithoutStars {
				out = append(out, next)
				addedPatternWithoutStars = true
			}
			continue
		}

		// Push next to the the out list and append a list of patterns
		// to the remain list for the next run by sequentially
		// substituting each double star pattern with a slash. For
		// example if next is "a/**/b/**c" the generated patterns will
		// be:
		// - a/b/**/c
		// - a/**/b/c
		out = append(out, next)
		for i := 1; i < len(parts); i++ {
			remaining = append(
				remaining,
				strings.Join(parts[:i], "/**/")+"/"+strings.Join(parts[i:], "/**/"),
			)
		}
	}

	return out
}
