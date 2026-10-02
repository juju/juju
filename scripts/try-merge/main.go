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
	// case "try-merge":
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

	// Map from display key (MM username or name) -> notification info
	peopleToNotify := map[string]*notifyInfo{}

	for _, commit := range badCommits {
		if num, ok := commitHasOpenPR(commit); ok {
			stderrf("DEBUG: skipping commit %s: has open PR #%d\n", commit.SHA, num)
			continue
		}

		// Try to find the PR that introduced this commit on the source branch
		prNumber, prAuthorEmail, prAuthorLogin, foundPR := findPRForCommit(commit)

		var email, name string
		if foundPR {
			name = prAuthorLogin
			email = prAuthorEmail
			if email == "" {
				// PR author email not public; fall back to commit author
				email = commit.AuthorEmail
			}
		} else {
			// Fallback to commit committer
			email = commit.CommitterEmail
			name = commit.CommitterName
		}

		if ignoreEmails.Contains(email) {
			stderrf("DEBUG: skipping commit %s: email on ignore list\n", commit.SHA)
			continue
		}

		// Determine the display key (MM username or name)
		var key string
		if mmUser, ok := emailToMMUser[email]; ok {
			key = "@" + mmUser
		} else {
			stderrf("WARNING: no MM username found for email %q\n", email)
			key = name
		}

		if _, exists := peopleToNotify[key]; !exists {
			peopleToNotify[key] = &notifyInfo{
				Name:      name,
				PRNumbers: set.NewStrings(),
			}
		}
		if foundPR && prNumber > 0 {
			peopleToNotify[key].PRNumbers.Add(fmt.Sprintf("#%d", prNumber))
		}
		peopleToNotify[key].Commits = append(peopleToNotify[key].Commits, commit)
	}

	if len(peopleToNotify) > 0 {
		printMessageWithPRs(peopleToNotify, os.Getenv("GITHUB_REPOSITORY"))
	}
}

// findOffendingCommits returns a list of commits that may be causing merge
// conflicts. This only works if Git is currently inside a failed merge.
func findOffendingCommits() []commitInfo {
	// Call `git log` to get commit info
	gitLogRes := execute(executeArgs{
		command: "git",
		args: []string{
			"log",
			// Restrict to commits which are present in source branch, but not target
			fmt.Sprintf("%s..%s", targetBranch, sourceBranch),
			"--merge",     // show refs that touch files having a conflict
			"--no-merges", // ignore merge commits
			"--format=" + gitLogFormat,
		},
		dir: gitDir,
	})
	handleExecuteError(gitLogRes)
	stderrf("DEBUG: offending commits are\n%s\n", gitLogRes.stdout)

	var commits []commitInfo
	for line := range strings.SplitSeq(string(gitLogRes.stdout), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 6)
		if len(parts) < 6 {
			stderrf("WARNING: skipping malformed git log line: %q\n", line)
			continue
		}
		commits = append(commits, commitInfo{
			SHA:            parts[0],
			AuthorName:     parts[1],
			AuthorEmail:    parts[2],
			CommitterName:  parts[3],
			CommitterEmail: parts[4],
			CommitMessage:  parts[5],
		})
	}
	return commits
}

var gitLogFormat = "%H\t%an\t%ae\t%cn\t%ce\t%s"

type commitInfo struct {
	SHA            string `json:"sha"`
	AuthorName     string `json:"authorName"`
	AuthorEmail    string `json:"authorEmail"`
	CommitterName  string `json:"committerName"`
	CommitterEmail string `json:"committerEmail"`
	CommitMessage  string `json:"commitMessage"`
}

type prInfo struct {
	Number int    `json:"number"`
	State  string `json:"state"`
}

// ghPRInfo represents a PR returned by the GitHub API endpoint
// /repos/{owner}/{repo}/commits/{sha}/pulls
type ghPRInfo struct {
	Number   int    `json:"number"`
	State    string `json:"state"`
	MergedAt string `json:"merged_at"`
	User     struct {
		Login string `json:"login"`
		Email string `json:"email"`
	} `json:"user"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// notifyInfo holds information about a person to notify for merge conflicts,
// including the PR numbers and commits they are responsible for.
type notifyInfo struct {
	Name      string
	PRNumbers set.Strings
	Commits   []commitInfo
}

// Check if there is already an open merge containing this commit. If so,
// we don't need to notify.
func commitHasOpenPR(commit commitInfo) (prNumber int, ok bool) {
	ghRes := execute(executeArgs{
		command: "gh",
		args: []string{
			"pr", "list",
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

// findPRForCommit finds the merged PR that introduced a commit on the source
// branch. Returns the PR number, author email, author login, and true if
// found.
func findPRForCommit(commit commitInfo) (prNumber int, prAuthorEmail string, prAuthorLogin string, ok bool) {
	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		return -1, "", "", false
	}

	ghRes := execute(executeArgs{
		command: "gh",
		args: []string{
			"api",
			fmt.Sprintf("/repos/%s/commits/%s/pulls", repo, commit.SHA),
		},
		dir: gitDir,
	})
	if ghRes.runError != nil {
		stderrf("WARNING: couldn't find PRs for commit %s: %v\n", commit.SHA, ghRes.runError)
		return -1, "", "", false
	}

	var prList []ghPRInfo
	if err := json.Unmarshal(ghRes.stdout, &prList); err != nil {
		stderrf("WARNING: couldn't parse PR list for commit %s: %v\n", commit.SHA, err)
		return -1, "", "", false
	}

	for _, pr := range prList {
		if pr.Base.Ref != sourceBranch {
			continue
		}
		if pr.MergedAt == "" {
			continue
		}
		return pr.Number, pr.User.Email, pr.User.Login, true
	}
	return -1, "", "", false
}

func printMessageWithPRs(peopleToNotify map[string]*notifyInfo, repo string) {
	var personBlocks []string
	for key, info := range peopleToNotify {
		var personLine string
		if info.PRNumbers.IsEmpty() {
			personLine = fmt.Sprintf("- %s", key)
		} else {
			prList := strings.Join(info.PRNumbers.SortedValues(), ", ")
			personLine = fmt.Sprintf("- %s (%s)", key, prList)
		}

		var commitLines []string
		for _, c := range info.Commits {
			shortSHA := c.SHA[:7]
			commitURL := fmt.Sprintf("%s/%s/commit/%s", os.Getenv("GITHUB_SERVER_URL"), repo, c.SHA)
			commitLines = append(commitLines, fmt.Sprintf("  - `%s` %s ([commit](%s))", shortSHA, c.CommitMessage, commitURL))
		}

		block := personLine
		if len(commitLines) > 0 {
			block += "\n" + strings.Join(commitLines, "\n")
		}
		personBlocks = append(personBlocks, block)
	}

	messageData := struct {
		SourceBranch, TargetBranch, Details string
	}{
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		Details:      strings.Join(personBlocks, "\n"),
	}

	tmpl, err := template.New("msg").Parse(
		"🤖 **Beep boop! Merge conflict detected!**\n" +
			"📍 **Source branch:** `{{.SourceBranch}}`\n\n" +
			"⚠️ The following changes on `{{.SourceBranch}}` have merge conflicts with `{{.TargetBranch}}`:\n" +
			"{{.Details}}\n\n" +
			"Please merge `{{.SourceBranch}}` into `{{.TargetBranch}}` and resolve the conflicts.",
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
