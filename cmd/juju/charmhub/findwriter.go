package charmhub

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/cmd/output"
)

func makeFindWriter(ctx *cmd.Context, in []charmhub.FindResponse) Printer {
	writer := findWriter{
		w:        ctx.Stdout,
		warningf: ctx.Warningf,
		in:       in,
	}
	return writer
}

type findWriter struct {
	warningf Log
	w        io.Writer
	in       []charmhub.FindResponse
}

func (f findWriter) Print() error {
	buffer := bytes.NewBufferString("")

	tw := output.TabWriter(buffer)

	fmt.Fprintf(tw, "Name\tVersion\tPublisher\tNotes\tSummary\n")
	for _, result := range f.in {
		entity := result.Entity

		// To ensure we don't break the tabular output, we select the first line
		// from the summary and output the first one.
		scanner := bufio.NewScanner(bytes.NewBufferString(strings.TrimSpace(entity.Summary)))
		scanner.Split(bufio.ScanLines)

		var summary string
		for scanner.Scan() {
			summary = scanner.Text()
			break
		}
		if err := scanner.Err(); err != nil {
			f.warningf("%v", errors.Annotate(err, "could not gather summary"))
		}

		version := result.DefaultRelease.Revision.Version

		var publisher string
		if p, ok := entity.Publisher["display-name"]; ok {
			publisher = p
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t-\t%s\n", result.Name, version, publisher, summary)
	}

	if err := tw.Flush(); err != nil {
		f.warningf("%v", errors.Annotate(err, "could not flush data to buffer"))
	}

	_, err := fmt.Fprintf(f.w, "%s\n", buffer.String())
	return err
}
