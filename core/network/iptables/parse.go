// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build ignore

package iptables

import (
	"log"
	"os"

	"github.com/kr/pretty"
)

func main() {
	rules, err := iptables.ParseIngressRules(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	pretty.Println(rules)
}
