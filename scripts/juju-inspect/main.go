// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/juju/scripts/juju-inspect/rules"
	"gopkg.in/yaml.v2"
)

func main() {
	files := os.Args[1:]
	if len(files) == 0 {
		log.Fatal("expected at least on file")
	}
	allRules := []Rule{
		rules.NewRaftRule(),
		rules.NewMongoRule(),
		rules.NewPubsubRule(),
		rules.NewManifoldsRule(),
		rules.NewStartCountRule(),
	}

	if len(files) == 1 {
		matches, err := filepath.Glob(files[0])
		if err == nil && len(matches) > 0 {
			files = matches
		}
	}

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			log.Fatal(err)
		}

		row1, _, err := bufio.NewReader(f).ReadLine()
		if err != nil {
			log.Fatal(err)
		}

		_, err = f.Seek(int64(len(row1)), io.SeekStart)
		if err != nil {
			log.Fatal(err)
		}

		var report rules.Report
		if err := yaml.NewDecoder(f).Decode(&report); err != nil {
			log.Fatal(err)
		}

		// TODO (pick a better name somehow - agent.name?)
		name := report.Manifolds["agent"].Report.Agent
		for _, rule := range allRules {
			rule.Run(name, report)
		}
	}

	fmt.Println("")
	fmt.Println("Analysis of Engine Report:")
	fmt.Println("")
	for _, rule := range allRules {
		fmt.Println(rule.Summary())

		analysis := rule.Analyse()

		buf := new(bytes.Buffer)
		scanner := bufio.NewScanner(strings.NewReader(analysis))
		for scanner.Scan() {
			fmt.Fprintf(buf, "\t%s\n", scanner.Text())
		}

		fmt.Printf("%s\n", buf.String())
	}
}

type Rule interface {
	Run(string, rules.Report)
	Summary() string
	Analyse() string
}
