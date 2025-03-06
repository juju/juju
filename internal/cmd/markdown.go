// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/gnuflag"
)

// InfoCommand is a subset of Command methods needed to print the Markdown
// document. In particular, all these methods are "static", hence should not
// do anything scary or destructive.
type InfoCommand interface {
	// Info returns information about the Command.
	Info() *Info
	// SetFlags adds command specific flags to the flag set.
	SetFlags(f *gnuflag.FlagSet)
}

// MarkdownOptions configures the output of the PrintMarkdown function.
type MarkdownOptions struct {
	// Title defines the title to print at the top of the document. If this
	// field is empty, no title will be printed.
	Title string
	// UsagePrefix will be printed before the command usage (for example, the
	// name of the supercommand).
	UsagePrefix string
	// LinkForCommand maps each "peer command" name (e.g. see also commands) to
	// the link target for that command (e.g. a section of the Markdown doc, or
	// a webpage).
	LinkForCommand func(string) string
	// LinkForSubcommand maps each sub-command name to the link target for that
	//command (e.g. a section of the Markdown doc, or a webpage).
	LinkForSubcommand func(string) string
}

// PrintMarkdown prints Markdown documentation about the given command to the
// given io.Writer. The MarkdownOptions can be provided to customise the
// output.
func PrintMarkdown(w io.Writer, cmd InfoCommand, opts MarkdownOptions) error {
	// We will write the document to a bytes.Buffer, then copy it over to the
	// specified io.Writer. This saves us having to check errors on every
	// single write - we can just check at the end when we copy over.
	var doc bytes.Buffer

	if opts.Title != "" {
		fmt.Fprintf(&doc, "# %s\n\n", opts.Title)
	}

	info := cmd.Info()

	// See Also
	if len(info.SeeAlso) > 0 {
		printSeeAlso(&doc, info.SeeAlso, opts.LinkForCommand)
	}

	if len(info.Aliases) > 0 {
		fmt.Fprint(&doc, "**Aliases:** ")
		fmt.Fprint(&doc, strings.Join(info.Aliases, ", "))
		fmt.Fprintln(&doc)
		fmt.Fprintln(&doc)
	}

	// Summary
	fmt.Fprintln(&doc, "## Summary")
	fmt.Fprintln(&doc, info.Purpose)
	fmt.Fprintln(&doc)

	// Usage
	if strings.TrimSpace(info.Args) != "" {
		fmt.Fprintln(&doc, "## Usage")
		fmt.Fprintf(&doc, "```")
		fmt.Fprint(&doc, opts.UsagePrefix)
		fmt.Fprintf(&doc, "%s [%ss] %s", info.Name, getFlagsName(info.FlagKnownAs), info.Args)
		fmt.Fprintf(&doc, "```")
		fmt.Fprintln(&doc)
		fmt.Fprintln(&doc)
	}

	// Options
	printFlags(&doc, cmd)

	// Examples
	if info.Examples != "" {
		fmt.Fprintln(&doc, "## Examples")
		fmt.Fprintln(&doc, info.Examples)
		fmt.Fprintln(&doc)
	}

	// Details
	if info.Doc != "" {
		fmt.Fprintln(&doc, "## Details")
		fmt.Fprintln(&doc, EscapeMarkdown(info.Doc))
		fmt.Fprintln(&doc)
	}

	if len(info.Subcommands) > 0 {
		printSubcommands(&doc, info.Subcommands, opts.LinkForSubcommand)
	}

	_, err := io.Copy(w, &doc)
	if err != nil {
		return fmt.Errorf("writing Markdown: %w", err)
	}
	return nil
}

func printSeeAlso(
	w io.Writer,
	seeAlso []string,
	linkForCommand func(string) string,
) {
	fmt.Fprint(w, "> See also: ")

	for i, cmdName := range seeAlso {
		fmt.Fprint(w, markdownLink(cmdName, linkForCommand))

		// Separate command names by commas
		if i < len(seeAlso)-1 {
			fmt.Fprint(w, ", ")
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)
}

// getFlagsName returns the default name for a command's flags, if this is not
// defined in the info.
func getFlagsName(fka string) string {
	if fka == "" {
		return "option"
	}
	return fka
}

func printFlags(w io.Writer, cmd InfoCommand) {
	info := cmd.Info()

	flagKnownAs := getFlagsName(info.FlagKnownAs)
	f := gnuflag.NewFlagSetWithFlagKnownAs(info.Name, gnuflag.ContinueOnError, flagKnownAs)
	cmd.SetFlags(f)

	// group together all flags for a given value, meaning that flag which sets the same value are
	// grouped together and displayed with the same description, as below:
	//
	// -s, --short, --alternate-string | default value | some description.
	flags := make(map[interface{}]flagsByLength)
	f.VisitAll(func(f *gnuflag.Flag) {
		flags[f.Value] = append(flags[f.Value], f)
	})
	if len(flags) == 0 {
		// No flags, so we won't print this section
		return
	}

	// sort the output flags by shortest name for each group.
	// Caution: this mean that description/default value displayed in documentation will
	// be the one of the shortest alias. Other will be discarded. Be careful to have the same default
	// values between each alias, and put the description on the shortest alias.
	var byName flagsByName
	for _, fl := range flags {
		sort.Sort(fl)
		byName = append(byName, fl)
	}
	sort.Sort(byName)

	fmt.Fprintln(w, "### Options")
	fmt.Fprintln(w, "| Flag | Default | Usage |")
	fmt.Fprintln(w, "| --- | --- | --- |")

	for _, fs := range byName {
		// Collect all flag aliases (usually a short one and a plain one, like -v / --verbose)
		formattedFlags := ""
		for i, f := range fs {
			if i > 0 {
				formattedFlags += ", "
			}
			if len(f.Name) == 1 {
				formattedFlags += fmt.Sprintf("`-%s`", f.Name)
			} else {
				formattedFlags += fmt.Sprintf("`--%s`", f.Name)
			}
		}
		// display all the flags aliases and the default value and description of the shortest one.
		// Escape Markdown in description in order to display it cleanly in the final documentation.
		fmt.Fprintf(w, "| %s | %s | %s |\n", formattedFlags,
			EscapeMarkdown(fs[0].DefValue),
			strings.ReplaceAll(EscapeMarkdown(fs[0].Usage), "\n", " "),
		)
	}
	fmt.Fprintln(w)
}

// flagsByLength is a slice of flags implementing sort.Interface,
// sorting primarily by the length of the flag, and secondarily
// alphabetically.
type flagsByLength []*gnuflag.Flag

func (f flagsByLength) Less(i, j int) bool {
	s1, s2 := f[i].Name, f[j].Name
	if len(s1) != len(s2) {
		return len(s1) < len(s2)
	}
	return s1 < s2
}
func (f flagsByLength) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
func (f flagsByLength) Len() int {
	return len(f)
}

// flagsByName is a slice of slices of flags implementing sort.Interface,
// alphabetically sorting by the name of the first flag in each slice.
type flagsByName [][]*gnuflag.Flag

func (f flagsByName) Less(i, j int) bool {
	return f[i][0].Name < f[j][0].Name
}
func (f flagsByName) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
func (f flagsByName) Len() int {
	return len(f)
}

func printSubcommands(
	w io.Writer,
	subcommands map[string]string,
	linkForSubcommand func(string) string,
) {
	sorted := []string{}
	for name := range subcommands {
		if isDefaultCommand(name) {
			continue
		}
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	if len(sorted) > 0 {
		fmt.Fprintln(w, "## Subcommands")
		for _, name := range sorted {
			fmt.Fprint(w, "- ")
			fmt.Fprint(w, markdownLink(name, linkForSubcommand))
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	}
}

// markdownLink uses the provided linker function to generate a Markdown
// hyperlink for the given key. It attempts to call the linker function on the
// given key to get the link target. If the function is nil or the output is
// empty, just the key (as a non-link) will be returned.
func markdownLink(key string, linker func(string) string) string {
	var target string
	if linker != nil {
		target = linker(key)
	}

	if target == "" {
		// We don't have a link target for this key, so just return the key.
		return key
	} else {
		return fmt.Sprintf("[%s](%s)", key, target)
	}
}

// EscapeMarkdown returns a copy of the input string, in which certain special
// Markdown characters (e.g. < > |) are escaped. These characters can otherwise
// cause the Markdown to display incorrectly if not escaped.
func EscapeMarkdown(raw string) string {
	escapeSeqs := map[rune]string{
		'<': "&lt;",
		'>': "&gt;",
		'&': "&amp;",
		'|': "&#x7c;",
	}

	var escaped strings.Builder
	escaped.Grow(len(raw))

	lines := strings.Split(raw, "\n")
	inTripleBacktickBlock := false

	for i, line := range lines {
		// Check if we're entering or leaving a triple backtick code block
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inTripleBacktickBlock = !inTripleBacktickBlock
			escaped.WriteString(line)
		} else if inTripleBacktickBlock || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			// Inside a code block - don't escape anything
			// Code blocks can be indented with four spaces or a tab
			escaped.WriteString(line)
		} else {
			// Keep track of whether we are inside a code span `...`
			// If so, don't escape characters
			insideCodeSpan := false

			for _, c := range line {
				if c == '`' {
					insideCodeSpan = !insideCodeSpan
				}

				if !insideCodeSpan {
					if escapeSeq, ok := escapeSeqs[c]; ok {
						escaped.WriteString(escapeSeq)
						continue
					}
				}
				escaped.WriteRune(c)
			}
		}

		if i < len(lines)-1 {
			escaped.WriteRune('\n')
		}
	}

	return escaped.String()
}
