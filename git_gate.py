#!/usr/bin/python
"""Merge gating script for git go projects."""

from __future__ import print_function

import argparse
import os
import subprocess
import sys

from utility import (
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


def go_test(gopath, project, project_url, project_ref, merge_url, merge_ref,
            dependencies):
    """Download, build and test a go package."""
    goenv = dict(os.environ)
    goenv["GOPATH"] = gopath
    go = SubcommandRunner("go", goenv)
    git = SubcommandRunner("git")
    directory = os.path.join(gopath, "src", project)
    if project_url:
        print("Cloning {} from {}".format(project, project_url))
        git("clone", project_url, directory)
    else:
        print("Getting {} using go".format(project))
        go("get", "-v", project)
    os.chdir(directory)
    if project_ref:
        print("Switching repository to {}".format(project_ref))
        git("checkout", project_ref)
    if merge_url:
        print("Merging {} ref {}".format(merge_url, merge_ref))
        git("fetch", merge_url, merge_ref)
        git("merge", "--no-ff", "-m", "Merged " + merge_ref, "FETCH_HEAD")
    for dep in dependencies:
        print("Getting {} using go".format(dep))
        go("get", "-v", dep)
    go("build", project + "/...")
    go("test", project + "/...")


def parse_args(args=None):
    """Parse arguments for gating script."""
    parser = argparse.ArgumentParser()
    project_group = parser.add_argument_group()
    project_group.add_argument("--project", required=True,
        help="Go import path of package to test.")
    project_group.add_argument("--project-url",
        help="URL to git repository of package.")
    project_group.add_argument("--project-ref",
        help="Branch name or tag to use as basis.")
    merge_group = parser.add_argument_group()
    merge_group.add_argument("--merge-url",
        help="URL to git repository to merge before testing.")
    merge_group.add_argument("--merge-ref", default="HEAD",
        help="Branch name or tag to merge before testing.")
    dep_group = parser.add_mutually_exclusive_group()
    dep_group.add_argument("--dependency", "-d", action="append", default=[],
        help="Go import path of package needed to for build or testing.")
    # GZ: Add dependencies.tsv argument option
    return parser.parse_args(args)


def main():
    args = parse_args()
    with temp_dir() as d:
        try:
            go_test(d, args.project, args.project_url, args.project_ref,
                args.merge_url, args.merge_ref, args.dependency)
        except SubcommandError as err:
            print(err, file=sys.stderr)
            return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
