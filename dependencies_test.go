// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gc "launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type dependenciesTest struct{}

var _ = gc.Suite(&dependenciesTest{})

func (*dependenciesTest) TestDependenciesTsvFormat(c *gc.C) {
	content, err := ioutil.ReadFile("dependencies.tsv")
	c.Assert(err, gc.IsNil)

	for _, line := range strings.Split(string(content), "\n") {
		if line == "" {
			continue
		}
		segments := strings.Split(line, "\t")
		c.Assert(segments, gc.HasLen, 4)
	}
}

func (*dependenciesTest) TestGodepsIsRight(c *gc.C) {
	f, err := os.Open("dependencies.tsv")
	c.Assert(err, gc.IsNil)
	defer f.Close()

	gopath := os.Getenv("GOPATH")
	gopath = strings.Split(gopath, fmt.Sprintf("%v", os.PathListSeparator))[0]

	godeps, err := exec.LookPath("godeps")
	if err != nil {
		godeps = filepath.Join(gopath, "bin", "godeps")
		if _, err := os.Stat(godeps); err != nil {
			c.Skip("Godeps not found in $PATH or $GOPATH/bin")
		}
	}

	cmd := exec.Command(godeps, "-t", "github.com/juju/juju/...")
	output, err := cmd.StdoutPipe()
	stderr, err := cmd.StderrPipe()
	c.Assert(err, gc.IsNil)
	err = cmd.Start()
	c.Assert(err, gc.IsNil)
	defer func() {
		out, err := ioutil.ReadAll(stderr)
		if err2 := cmd.Wait(); err2 != nil {
			if err != nil {
				c.Fatalf("Error running godeps: %s", err2)
			}
			c.Fatal(string(out))
		}
	}()

	if err := diff(output, f); err != nil {
		c.Fatal(err)
	}
}

func diff(exp, act io.Reader) error {
	// this whole monstrosity loops through the file contents and ensures each
	// line is the same as in the godeps output.
	actscan := bufio.NewScanner(act)
	outscan := bufio.NewScanner(exp)
	for actscan.Scan() {
		if !outscan.Scan() {
			// Scanning godeps output ended before the file contents ended.

			if err := outscan.Err(); err != nil {
				return fmt.Errorf("Error reading from godeps output: %v", err)
			}

			// Since there's no error, this means there are dependencies in the
			// file that aren't actual dependnecies.
			errs := []string{
				"dependencies.tsv contains entries not reported by godeps: "}
			errs = append(errs, actscan.Text())
			for actscan.Scan() {
				errs = append(errs, actscan.Text())
			}
			if err := actscan.Err(); err != nil {
				return fmt.Errorf("Error reading from dependencies.tsv: %v", err)
			}
			return errors.New(strings.Join(errs, "\n"))
		}
		output := outscan.Text()
		filedata := actscan.Text()
		if output != filedata {
			return fmt.Errorf("Godeps output does not match dependencies.tsv:\ngodeps: %s\n  .tsv: %s", output, filedata)
		}
	}
	if err := actscan.Err(); err != nil {
		return fmt.Errorf("Error reading from dependencies.tsv: %v", err)
	}

	if outscan.Scan() {
		// dependencies.tsv scan ended before godeps output ended.
		// This means there are dependendencies missing in the file.
		errs := []string{
			"Godeps reports dependencies not contained in dependencies.tsv: "}
		errs = append(errs, outscan.Text())
		for outscan.Scan() {
			errs = append(errs, outscan.Text())
		}
		if err := outscan.Err(); err != nil {
			return fmt.Errorf("Error reading from godeps output: %v", err)
		}
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}
