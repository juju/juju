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
    project_ellipsis = args.project + "/..."
    directory = os.path.join(gopath, "src", args.project)
    if args.project_url:
        print_now("Cloning {} from {}".format(args.project, args.project_url))
        git("clone", args.project_url, directory)
    if args.go_get_all:
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
    if args.dependencies:
        for dep in args.dependencies:
            print_now("Getting {} and dependencies using go".format(dep))
            go("get", "-v", "-d", dep)
    go("build", project_ellipsis)
    go("test", project_ellipsis)


def parse_args(args=None):
    """Parse arguments for gating script."""
    parser = argparse.ArgumentParser()
    project_group = parser.add_argument_group()
    project_group.add_argument(
        "--project", required=True, help="Go import path of package to test.")
    project_group.add_argument(
        "--project-url", help="URL to git repository of package.")
    project_group.add_argument(
        "--project-ref", help="Branch name or tag to use as basis.")
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
    # GZ: Add dependencies.tsv argument option
    args = parser.parse_args(args)
    if args.project_url is None and not args.go_get_all:
        parser.exit("Must supply either --project-url or --go-get-all")
    return args


def main():
    args = parse_args()
    with temp_dir() as d:
        try:
            go_test(args, d)
        except SubcommandError as err:
            print(err, file=sys.stderr)
            return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
