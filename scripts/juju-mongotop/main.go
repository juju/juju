// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bufio"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/olekukonko/tablewriter"
)

func main() {
	var viewable int
	var sortOn string
	flag.IntVar(&viewable, "viewable", 20, "number of viewable in a table")
	flag.StringVar(&sortOn, "sort-on", "total", "sort either on total, read or write (default is write)")
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		log.Fatal("expected at least on file")
	}

	if len(files) == 1 {
		matches, err := filepath.Glob(files[0])
		if err == nil && len(matches) > 0 {
			files = matches
		}
	}

	var lines []Line
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			log.Fatal(err)
		}

		buf := bufio.NewReader(f)
		for {
			bytes, _, err := buf.ReadLine()
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Fatal(err)
			}
			if len(bytes) == 0 {
				continue
			}
			if headerRegex.Match(bytes) {
				continue
			}
			matches := lineRegex.FindSubmatch(bytes)

			total, err := time.ParseDuration(string(matches[2]))
			if err != nil {
				log.Fatal(err)
			}
			read, err := time.ParseDuration(string(matches[3]))
			if err != nil {
				log.Fatal(err)
			}
			write, err := time.ParseDuration(string(matches[4]))
			if err != nil {
				log.Fatal(err)
			}

			lines = append(lines, Line{
				Namespace: string(matches[1]),
				Total:     total,
				Read:      read,
				Write:     write,
			})
		}
	}

	sort.Slice(lines, func(i, j int) bool {
		switch sortOn {
		case "read":
			return lines[i].Read > lines[j].Read
		case "write":
			return lines[i].Write > lines[j].Write
		default:
			return lines[i].Total > lines[j].Total
		}
	})

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ns", "total", "read", "write"})

	max := viewable
	if len(lines) < max {
		max = len(lines)
	}

	for i := 0; i < max; i++ {
		line := lines[i]
		table.Append([]string{
			line.Namespace,
			line.Total.String(),
			line.Read.String(),
			line.Write.String(),
		})
	}

	table.Render()
}

var (
	headerRegex = regexp.MustCompile(`^\s+ns\s+total\s+read\s+write\s+[0-9\-\:TZ]+$`)
	lineRegex   = regexp.MustCompile(`^^\s*([a-zA-Z0-9\.\-]+)\s+([0-9]+ms)\s+([0-9]+ms)\s+([0-9]+ms)\s+$`)
)

type Line struct {
	Namespace          string
	Total, Read, Write time.Duration
}
