// Copyright 2012-2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/gnuflag"
)

const (
	DocumentationFileName      = "documentation.md"
	DocumentationIndexFileName = "index.md"
)

var doc string = `
This command generates a markdown formatted document with all the commands, their descriptions, arguments, and examples.
`

var documentationExamples = `
    juju documentation
    juju documentation --split
    juju documentation --split --no-index --out /tmp/docs

To render markdown documentation using a list of existing
commands, you can use a file with the following syntax

    command1: id1
    command2: id2
    commandN: idN

For example:

    add-cloud: 1183
    add-secret: 1284
    remove-cloud: 4344

Then, the urls will be populated using the ids indicated
in the file above.

    juju documentation --split --no-index --out /tmp/docs --discourse-ids /tmp/docs/myids
`

type documentationCommand struct {
	CommandBase
	super   *SuperCommand
	out     string
	noIndex bool
	split   bool
	url     string
	idsPath string
	// ids is contains a numeric id of every command
	// add-cloud: 1112
	// remove-user: 3333
	// etc...
	ids map[string]string
	// reverseAliases maintains a reverse map of the alias and the
	// targeting command. This is used to find the ids corresponding
	// to a given alias
	reverseAliases map[string]string
}

func (c *documentationCommand) Info() *Info {
	return &Info{
		Name:     "documentation",
		Args:     "--out <target-folder> --no-index --split --url <base-url> --discourse-ids <filepath>",
		Purpose:  "Generate the documentation for all commands",
		Doc:      doc,
		Examples: documentationExamples,
	}
}

// SetFlags adds command specific flags to the flag set.
func (c *documentationCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.out, "out", "", "Documentation output folder if not set the result is displayed using the standard output")
	f.BoolVar(&c.noIndex, "no-index", false, "Do not generate the commands index")
	f.BoolVar(&c.split, "split", false, "Generate a separate Markdown file for each command")
	f.StringVar(&c.url, "url", "", "Documentation host URL")
	f.StringVar(&c.idsPath, "discourse-ids", "", "File containing a mapping of commands and their discourse ids")
}

func (c *documentationCommand) Run(ctx *Context) error {
	if c.split {
		if c.out == "" {
			return errors.New("when using --split, you must set the output folder using --out=<folder>")
		}
		return c.dumpSeveralFiles()
	}
	return c.dumpOneFile(ctx)
}

// dumpOneFile is invoked when the output is contained in a single output
func (c *documentationCommand) dumpOneFile(ctx *Context) error {
	var writer io.Writer
	if c.out != "" {
		_, err := os.Stat(c.out)
		if err != nil {
			return err
		}

		target := fmt.Sprintf("%s/%s", c.out, DocumentationFileName)

		f, err := os.Create(target)
		if err != nil {
			return err
		}
		defer f.Close()
		writer = f
	} else {
		writer = ctx.Stdout
	}

	return c.dumpEntries(writer)
}

// getSortedListCommands returns an array with the sorted list of
// command names
func (c *documentationCommand) getSortedListCommands() []string {
	// sort the commands
	sorted := make([]string, len(c.super.subcmds))
	i := 0
	for k := range c.super.subcmds {
		sorted[i] = k
		i++
	}
	sort.Strings(sorted)
	return sorted
}

func (c *documentationCommand) computeReverseAliases() {
	c.reverseAliases = make(map[string]string)

	for name, content := range c.super.subcmds {
		for _, alias := range content.command.Info().Aliases {
			c.reverseAliases[alias] = name
		}
	}

}

// dumpSeveralFiles is invoked when every command is dumped into
// a separated entity
func (c *documentationCommand) dumpSeveralFiles() error {
	if len(c.super.subcmds) == 0 {
		fmt.Printf("No commands found for %s", c.super.Name)
		return nil
	}

	// Attempt to create output directory. This will fail if:
	// - we don't have permission to create the dir
	// - a file already exists at the given path
	err := os.MkdirAll(c.out, os.ModePerm)
	if err != nil {
		return err
	}

	if c.idsPath != "" {
		// get the list of ids
		c.ids, err = c.readFileIds(c.idsPath)
		if err != nil {
			return err
		}
	}

	// create index if indicated
	if !c.noIndex {
		target := fmt.Sprintf("%s/%s", c.out, DocumentationIndexFileName)
		f, err := os.Create(target)
		if err != nil {
			return err
		}

		err = c.writeIndex(f)
		if err != nil {
			return fmt.Errorf("writing index: %w", err)
		}
		f.Close()
	}

	return c.writeDocs(c.out, []string{c.super.Name}, true)
}

// writeDocs (recursively) writes docs for all commands in the given folder.
func (c *documentationCommand) writeDocs(folder string, superCommands []string, printDefaultCommands bool) error {
	c.computeReverseAliases()

	for name, ref := range c.super.subcmds {
		if !printDefaultCommands && isDefaultCommand(name) {
			continue
		}

		if ref.alias != "" {
			continue
		}

		commandSeq := append(superCommands, name)

		sc, isSuperCommand := ref.command.(*SuperCommand)
		if !isSuperCommand || (isSuperCommand && !sc.SkipCommandDoc) {
			target := fmt.Sprintf("%s.md", strings.Join(commandSeq[1:], "_"))
			if err := c.writeDoc(folder, target, ref, commandSeq); err != nil {
				return err
			}
		}

		// Handle subcommands
		if !isSuperCommand {
			continue
		}

		if err := sc.documentation.writeDocs(folder, commandSeq, false); err != nil {
			return err
		}
	}

	return nil
}

func (c *documentationCommand) writeDoc(folder, target string, ref commandReference, commandSeq []string) error {
	target = strings.ReplaceAll(target, " ", "_")
	target = filepath.Join(folder, target)

	f, err := os.Create(target)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	formatted, err := c.formatCommand(ref, false, commandSeq)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, formatted)
	if err != nil {
		return err
	}
	err = f.Sync()
	if err != nil {
		return err
	}
	return nil
}

func (c *documentationCommand) readFileIds(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := bufio.NewScanner(f)
	ids := make(map[string]string)
	for reader.Scan() {
		line := reader.Text()
		items := strings.Split(line, ":")
		if len(items) != 2 {
			return nil, fmt.Errorf("malformed line [%s]", line)
		}
		command := strings.TrimSpace(items[0])
		id := strings.TrimSpace(items[1])
		ids[command] = id
	}
	return ids, nil
}

func (c *documentationCommand) dumpEntries(w io.Writer) error {
	if len(c.super.subcmds) == 0 {
		fmt.Printf("No commands found for %s", c.super.Name)
		return nil
	}

	if !c.noIndex {
		err := c.writeIndex(w)
		if err != nil {
			return fmt.Errorf("writing index: %w", err)
		}
	}

	return c.writeSections(w, []string{c.super.Name}, true)
}

// writeSections (recursively) writes sections for all commands to the given file.
func (c *documentationCommand) writeSections(w io.Writer, superCommands []string, printDefaultCommands bool) error {
	sorted := c.getSortedListCommands()
	for _, name := range sorted {
		if !printDefaultCommands && isDefaultCommand(name) {
			continue
		}
		ref := c.super.subcmds[name]
		commandSeq := append(superCommands, name)

		// This is a bit messy, because we want to keep the order of the
		// documentation the same.
		sc, isSuperCommand := ref.command.(*SuperCommand)
		if !isSuperCommand || (isSuperCommand && !sc.SkipCommandDoc) {
			formatted, err := c.formatCommand(ref, true, commandSeq)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(w, "%s", formatted)
			if err != nil {
				return err
			}
		}

		// Handle subcommands
		if !isSuperCommand {
			continue
		}
		if err := sc.documentation.writeSections(w, commandSeq, false); err != nil {
			return err
		}
	}
	return nil
}

// writeIndex writes the command index to the specified writer.
func (c *documentationCommand) writeIndex(w io.Writer) error {
	_, err := fmt.Fprintf(w, "# Index\n")
	if err != nil {
		return err
	}

	listCommands := c.getSortedListCommands()
	for id, name := range listCommands {
		if isDefaultCommand(name) {
			continue
		}
		_, err = fmt.Fprintf(w, "%d. [%s](%s)\n", id, name, c.linkForCommand(name))
		if err != nil {
			return err
		}
		// TODO: handle subcommands ??
	}
	_, err = fmt.Fprintf(w, "---\n\n")
	return err
}

// Return the URL/location for the given command
func (c *documentationCommand) linkForCommand(cmd string) string {
	prefix := "#"
	if c.ids != nil {
		prefix = "/t/"
	}
	if c.url != "" {
		prefix = c.url + "/"
	}

	target, err := c.getTargetCmd(cmd)
	if err != nil {
		fmt.Printf("[ERROR] command [%s] has no id, please add it to the list\n", cmd)
		return ""
	}
	return prefix + target
}

// formatCommand returns a string representation of the information contained
// by a command in Markdown format. The title param can be used to set
// whether the command name should be a title or not. This is particularly
// handy when splitting the commands in different files.
func (c *documentationCommand) formatCommand(ref commandReference, title bool, commandSeq []string) (string, error) {
	var fmtedTitle string
	if title {
		fmtedTitle = strings.ToUpper(strings.Join(commandSeq[1:], " "))
	}

	opts := MarkdownOptions{
		Title:       fmtedTitle,
		UsagePrefix: strings.Join(commandSeq[:len(commandSeq)-1], " ") + " ",
		LinkForCommand: func(s string) string {
			prefix := "#"
			if c.ids != nil {
				prefix = "/t/"
			}
			if c.url != "" {
				prefix = c.url + "t/"
			}

			target, err := c.getTargetCmd(s)
			if err != nil {
				fmt.Println(err.Error())
			}
			return fmt.Sprintf("%s%s", prefix, target)
		},
		LinkForSubcommand: func(s string) string {
			return c.linkForCommand(strings.Join(append(commandSeq[1:], s), "_"))
		},
	}

	var buf bytes.Buffer
	err := PrintMarkdown(&buf, ref.command, opts)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// getTargetCmd is an auxiliary function that returns the target command or
// the corresponding id if available.
func (d *documentationCommand) getTargetCmd(cmd string) (string, error) {
	// no ids were set, return the original command
	if d.ids == nil {
		return cmd, nil
	}
	target, found := d.ids[cmd]
	if found {
		return target, nil
	} else {
		// check if this is an alias
		targetCmd, found := d.reverseAliases[cmd]
		fmt.Printf("use alias %s -> %s\n", cmd, targetCmd)
		if !found {
			// if we're working with ids, and we have to mmake the translation,
			// we need to have an id per every requested command
			return "", fmt.Errorf("requested id for command %s was not found", cmd)
		}
		return targetCmd, nil

	}
}
