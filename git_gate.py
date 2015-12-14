#!/usr/bin/python
"""Merge gating script for git go projects."""

from __future__ import print_function

import argparse
import os
import subprocess
import sys

from utility import (
    print_now,
    temp_dir,
)


class SubcommandError(Exception):

    def __init__(self, command, subcommand, error):
        self.command = command
        self.subcommand = subcommand
        self.error = error

    def __str__(self):
        return "Subprocess {} {} failed with code {}".format(
            self.command, self.subcommand, self.error.returncode)


class SubcommandRunner(object):

    def __init__(self, command, environ=None):
        self.command = command
        self.subprocess_kwargs = {}
        if environ is not None:
            self.subprocess_kwargs["env"] = environ

    def __call__(self, subcommand, *args):
        cmdline = [self.command, subcommand]
        cmdline.extend(args)
        try:
            subprocess.check_call(cmdline, **self.subprocess_kwargs)
        except subprocess.CalledProcessError as e:
            raise SubcommandError(self.command, subcommand, e)


def go_test(args, gopath):
    """Download, build and test a go package."""
    goenv = dict(os.environ)
    goenv["GOPATH"] = gopath
    go = SubcommandRunner("go", goenv)
    git = SubcommandRunner("git")

    final_project = args.project
    if args.feature_branch:
        final_project = from_feature_dir(args.project)

    project_ellipsis = final_project + "/..."
    directory = os.path.join(gopath, "src", final_project)

    if args.tsv_path:
        print_now("Getting and installing godeps")
        go("get", "-v", "-d", "launchpad.net/godeps/...")
        go("install", "launchpad.net/godeps/...")
    if args.project_url:
        print_now("Cloning {} from {}".format(final_project, args.project_url))
        git("clone", args.project_url, directory)
    if args.go_get_all and not (args.project_url and args.merge_url):
        print_now("Getting {} and dependencies using go".format(args.project))
        go("get", "-v", "-d", "-t", project_ellipsis)
    os.chdir(directory)
    if args.project_ref:
        print_now("Switching repository to {}".format(args.project_ref))
        git("checkout", args.project_ref)
    if args.merge_url:
        print_now("Merging {} ref {}".format(args.merge_url, args.merge_ref))
        git("fetch", args.merge_url, args.merge_ref)
        git("merge", "--no-ff", "-m", "Merged " + args.merge_ref, "FETCH_HEAD")
        if args.go_get_all:
            print_now("Updating {} dependencies using go".format(args.project))
            go("get", "-v", "-d", "-t", project_ellipsis)
    if args.dependencies:
        for dep in args.dependencies:
            print_now("Getting {} and dependencies using go".format(dep))
            go("get", "-v", "-d", dep)
    if args.tsv_path:
        tsv_path = os.path.join(gopath, "src", final_project, args.tsv_path)
        print_now("Getting dependencies using godeps from {}".format(tsv_path))
        godeps = SubcommandRunner(os.path.join(gopath, "bin", "godeps"), goenv)
        godeps("-u", tsv_path)
    go("build", project_ellipsis)
    go("test", project_ellipsis)


def from_feature_dir(directory):
    """
    For feature branches on repos that are versioned with gopkg.in,  we need to
    do some special handling, since the test code expects the branch name to be
    appended to the reponame with a ".".  However, for a feature branch, the
    branchname is different than the base gopkg.in branch.  To account for
    this, we use the convention of base_branch_name.featurename, and thus this
    code can know that it needs to strip out the featurename when locating the
    code on disk.

    Thus, the feature branch off of gopkg.in/juju/charm.v6 would be a branch
    named charm.v6.myfeature, which should end up in
    $GOPATH/src/gokpg.in/juju/charm.v6
    """
    name = os.path.basename(directory)
    parts = name.split(".")
    if len(parts) == 3:
        return directory[:-len(parts[2]) - 1]
    return directory


def parse_args(args=None):
    """Parse arguments for gating script."""
    parser = argparse.ArgumentParser()
    project_group = parser.add_argument_group()
    project_group.add_argument(
        "--keep", action="store_true",
        help="Do not remove working dir after testing.")
    project_group.add_argument(
        "--project", required=True, help="Go import path of package to test.")
    project_group.add_argument(
        "--project-url", help="URL to git repository of package.")
    project_group.add_argument(
        "--project-ref", help="Branch name or tag to use as basis.")
    project_group.add_argument(
        "--feature-branch", action="store_true",
        help="Use special handling for pending feature branches.")
    merge_group = parser.add_argument_group()
    merge_group.add_argument(
        "--merge-url", help="URL to git repository to merge before testing.")
    merge_group.add_argument(
        "--merge-ref", default="HEAD",
        help="Branch name or tag to merge before testing.")
    dep_group = parser.add_mutually_exclusive_group()
    dep_group.add_argument(
        "--dependencies", nargs="+",
        help="Any number of package import paths needed for build or testing.")
    dep_group.add_argument(
        "--go-get-all", action="store_true",
        help="Go import path of package needed to for build or testing.")
    dep_group.add_argument(
        "--tsv-path",
        help="Path to dependencies.tsv file relative to project dir.")
    args = parser.parse_args(args)
    if args.project_url is None and not args.go_get_all:
        parser.error("Must supply either --project-url or --go-get-all")
    if args.feature_branch and args.go_get_all:
        parser.error("Cannot use --feature-branch and --go-get-all together")
    return args


def main():
    args = parse_args()
    with temp_dir(keep=args.keep) as d:
        try:
            go_test(args, d)
        except SubcommandError as err:
            print(err, file=sys.stderr)
            return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
