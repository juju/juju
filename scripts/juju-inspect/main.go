// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/scripts/juju-inspect/rules"
)

func main() {
	var includeNested bool
	var startCountAmount int
	flag.BoolVar(&includeNested, "include-nested", false, "inlcude nested agents")
	flag.IntVar(&startCountAmount, "start-count-amount", 3, "number of start counts to show")
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		log.Fatal("expected at least on file")
	}
	allRules := []Rule{
		rules.NewRaftRule(),
		rules.NewMongoRule(),
		rules.NewPubsubRule(),
		rules.NewManifoldsRule(includeNested),
		rules.NewStartCountRule(includeNested, startCountAmount),
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

		// Engine reports aren't actually valid yaml files. Instead they have a
		// header that isn't a comment! This code exists to skip that line.
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

		agent := report.Manifolds["agent"]

		var out AgentReport
		if err := agent.UnmarshalReport(&out); err != nil {
			log.Fatal(err)
		}
		for _, rule := range allRules {
			if err := rule.Run(out.Agent, report); err != nil {
				fmt.Printf("Skipping %T, because of error: %v", rule, err)
			}
		}
	}

	fmt.Println("")
	fmt.Println("Analysis of Engine Report:")
	fmt.Println("")

	buf := new(bytes.Buffer)
	for _, rule := range allRules {
		rule.Write(buf)
	}
	fmt.Println(buf)
}

type Rule interface {
	Run(string, rules.Report) error
	Write(io.Writer)
}

type AgentReport struct {
	Agent   string            `yaml:"agent"`
	Version semversion.Number `yaml:"version"`
}
