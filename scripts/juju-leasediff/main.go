// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/juju/collections/set"
	"gopkg.in/yaml.v3"
)

func main() {
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		log.Fatal("expected at least on file")
	}

	reports := make([]Controller, len(files))
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			log.Fatal(err)
		}

		var report Controller
		if err := yaml.NewDecoder(f).Decode(&report); err != nil {
			log.Fatal(err)
		}

		reports[i] = report
	}

	// Process them to ensure that all leases are correctly held.
	sets := make([]set.Strings, len(reports))
	for i, report := range reports {
		set := set.NewStrings()
		for k, v := range report.ControllerLeases {
			set.Add(fmt.Sprintf("%s:%s", k, v.Holder))
		}
		for k, v := range report.ModelLeases {
			for app, lease := range v {
				set.Add(fmt.Sprintf("%s:%s:%s", k, app, lease.Holder))
			}
		}
		sets[i] = set
	}

	for i := 1; i < len(sets); i++ {
		a := sets[i-1]
		b := sets[i]

		x := a.Difference(b)
		y := b.Difference(a)

		if len(x) > 0 || len(y) > 0 {
			fmt.Println("Difference located:")
			fmt.Println(x)
			fmt.Println(y)
			return
		}

		for _, v0 := range a.SortedValues() {
			parts := strings.Split(v0, ":")
			if len(parts) == 2 {
				l0 := reports[i-1].ControllerLeases[parts[0]]
				l1 := reports[i].ControllerLeases[parts[0]]
				if l0.LeaseExpires != l1.LeaseExpires {
					fmt.Printf("Controller lease (%d - %d): %s:: %s - %s\n", i-1, i, parts[0], l0, l1)
				}
			} else if len(parts) == 3 {
				l0 := reports[i-1].ModelLeases[parts[0]][parts[1]]
				l1 := reports[i].ModelLeases[parts[0]][parts[1]]
				if l0.LeaseExpires != l1.LeaseExpires {
					fmt.Printf("Model lease (%d - %d): %s:: %s - %s\n", i-1, i, parts[0], l0, l1)
				}
			}
		}
	}
}

type Controller struct {
	ControllerLeases map[string]Lease            `yaml:"controller-leases"`
	ModelLeases      map[string]map[string]Lease `yaml:"model-leases"`
}

type Lease struct {
	Holder       string `yaml:"holder"`
	LeaseExpires string `yaml:"lease-expires"`
}
