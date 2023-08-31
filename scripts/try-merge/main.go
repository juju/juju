// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/juju/collections/set"
)

// Environment variables to configure script
var (
	sourceBranch string // branch containing changes (e.g. 2.9)
	targetBranch string // branch to merge into (e.g. 3.1)
	gitDir       string // location of checked out branch. Git commands will be run here

	emailToMMUser map[string]string // mapping of email address -> Mattermost username
	ignoreEmails  set.Strings       // email addresses to ignore (e.g. bot accounts)
)

func main() {
	// Get configuration from environment
	sourceBranch = os.Getenv("SOURCE_BRANCH")
	targetBranch = os.Getenv("TARGET_BRANCH")
	gitDir = os.Getenv("GIT_DIR")
	fillEmailToMMUserMap()
	fillIgnoreEmails()

	if len(os.Args) < 2 {
		fatalf("no command provided\n")
	}
	switch cmd := os.Args[1]; cmd {
	// TODO: migrate the merging logic from merge.yml to here
	//case "try-merge":
	//	tryMerge()
	case "errmsg":
		printErrMsg()
	default:
		fatalf("unrecognised command %q\n", cmd)
	}
}

// Get the contents of the EMAIL_TO_MM_USER environment variable, which is
// a JSON mapping of email addresses to Mattermost usernames. Parse this into
// the emailToMMUser map.
func fillEmailToMMUserMap() {
	emailToMMUser = map[string]string{}
	jsonMap := os.Getenv("EMAIL_TO_MM_USER")
	err := json.Unmarshal([]byte(jsonMap), &emailToMMUser)
	if err != nil {
		// No need to fail - we can still use the commit author name.
		// Just log a warning.
		stderrf("WARNING: couldn't parse EMAIL_TO_MM_USER: %v\n", err)
	}
}

// Get the contents of the IGNORE_EMAILS environment variable, which is
// a JSON list of email addresses to ignore / not notify. Parse this into
// the ignoreEmails set.
func fillIgnoreEmails() {
	jsonList := os.Getenv("IGNORE_EMAILS")

	var ignoreEmailsList []string
	err := json.Unmarshal([]byte(jsonList), &ignoreEmailsList)
	if err != nil {
		// No need to fail here
		stderrf("WARNING: couldn't parse IGNORE_EMAILS: %v\n", err)
	}

	ignoreEmails = set.NewStrings(ignoreEmailsList...)
}

// After a failed merge, generate a nice notification message that will be
// sent to Mattermost.
func printErrMsg() {
	// Check required env variables are set
	if sourceBranch == "" {
		fatalf("fatal: SOURCE_BRANCH not set\n")
	}
	if targetBranch == "" {
		fatalf("fatal: TARGET_BRANCH not set\n")
	}

	badCommits := findOffendingCommits()

	// Iterate through commits and find people to notify
	peopleToNotify := set.NewStrings()
	for _, commit := range badCommits {
		if ignoreEmails.Contains(commit.CommitterEmail) {
			stderrf("DEBUG: skipping commit %s: committer on ignore list\n", commit.SHA)
			continue
		}
		if num, ok := commitHasOpenPR(commit); ok {
			stderrf("DEBUG: skipping commit %s: has open PR #%d\n", commit.SHA, num)
			continue
		}

		_, ok := emailToMMUser[commit.CommitterEmail]
		if ok {
			peopleToNotify.Add("@" + emailToMMUser[commit.CommitterEmail])
		} else {
			// Don't have a username for this email - just use commit author name
			stderrf("WARNING: no MM username found for email %q\n", commit.CommitterEmail)
			peopleToNotify.Add(commit.CommitterName)
		}
	}

	if !peopleToNotify.IsEmpty() {
		printMessage(peopleToNotify)
	}
}

// findOffendingCommits returns a list of commits that may be causing merge
// conflicts. This only works if Git is currently inside a failed merge.
func findOffendingCommits() []commitInfo {
	// Call `git log` to get commit info
	gitLogRes := execute(executeArgs{
		command: "git",
		args: []string{"log",
			// Restrict to commits which are present in source branch, but not target
			fmt.Sprintf("%s..%s", targetBranch, sourceBranch),
			"--merge",     // show refs that touch files having a conflict
			"--no-merges", // ignore merge commits
			"--format=" + gitLogJSONFormat,
		},
		dir: gitDir,
	})
	handleExecuteError(gitLogRes)
	stderrf("DEBUG: offending commits are\n%s\n", gitLogRes.stdout)
	gitLogInfo := gitLogOutputToValidJSON(gitLogRes.stdout)

	var commits []commitInfo
	check(json.Unmarshal(gitLogInfo, &commits))
	return commits
}

var gitLogJSONFormat = `{"sha":"%H","authorName":"%an","authorEmail":"%ae","committerName":"%cn","committerEmail":"%ce"}`

// Transforms the output of `git log` into a valid JSON array.
func gitLogOutputToValidJSON(raw []byte) []byte {
	rawString := string(raw)
	lines := strings.Split(rawString, "\n")
	// Remove empty last line
	filteredLines := lines[:len(lines)-1]
	joinedLines := strings.Join(filteredLines, ",")
	array := "[" + joinedLines + "]"
	return []byte(array)
}

type commitInfo struct {
	SHA            string `json:"sha"`
	AuthorName     string `json:"authorName"`
	AuthorEmail    string `json:"authorEmail"`
	CommitterName  string `json:"committerName"`
	CommitterEmail string `json:"committerEmail"`
}

type prInfo struct {
	Number int    `json:"number"`
	State  string `json:"state"`
}

// Check if there is already an open merge containing this commit. If so,
// we don't need to notify.
func commitHasOpenPR(commit commitInfo) (prNumber int, ok bool) {
	ghRes := execute(executeArgs{
		command: "gh",
		args: []string{"pr", "list",
			"--search", commit.SHA,
			"--state", "all",
			"--base", targetBranch,
			"--json", "number,state",
		},
	})
	handleExecuteError(ghRes)

	prList := []prInfo{}
	check(json.Unmarshal(ghRes.stdout, &prList))

	for _, pr := range prList {
		// Check for merged PRs, just in case the merge PR landed while we've been
		// checking for conflicts.
		if pr.State == "OPEN" || pr.State == "MERGED" {
			return pr.Number, true
		}
	}
	return -1, false
}

func printMessage(peopleToNotify set.Strings) {
	messageData := struct{ TaggedUsers, SourceBranch, TargetBranch, LogsLink string }{
		TaggedUsers:  strings.Join(peopleToNotify.Values(), ", "),
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		LogsLink: fmt.Sprintf("https://github.com/%s/actions/runs/%s",
			os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID")),
	}

	tmpl, err := template.New("test").Parse(
		"{{.TaggedUsers}}: your recent changes to `{{.SourceBranch}}` have caused merge conflicts. " +
			"Please merge `{{.SourceBranch}}` into `{{.TargetBranch}}` and resolve the conflicts." +
			"[[logs]({{.LogsLink}})]",
	)
	check(err)
	check(tmpl.Execute(os.Stdout, messageData))
}

func check(err error) {
	if err != nil {
		stderrf("%#v\n", err)
		panic(err)
	}
}

// Print to stderr. Logging/debug info should go here, so that it is kept
// separate from the actual output.
func stderrf(f string, v ...any) {
	_, _ = fmt.Fprintf(os.Stderr, f, v...)
}

// Print to stderr and then exit.
func fatalf(f string, v ...any) {
	stderrf(f, v...)
	os.Exit(1)
}
