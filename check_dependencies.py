#!/usr/bin/python
"""Script for checking if a directory tree matches a dependencies list"""

from __future__ import print_function

import argparse
import os
import sys


def get_dependencies(filename):
    """Get path of each dependency from tsv file"""
    deps = set()
    with open(filename) as f:
        for line in f:
            deps.add(line.split("\t", 1)[0])
    return deps


def compare_dependencies(deps, srcdir):
    """Give the difference between expected deps and go src directory"""
    present = []
    unknown = []
    for r, ds, fs in os.walk(srcdir):
        path = os.path.relpath(r, srcdir)
        d = os.path.basename(r)
        if path in deps or ("." in d and path.rsplit(".", 1)[0] in deps):
            present.append(path)
            del ds[:]
        elif fs:
            unknown.append(path)
            del ds[:]
    return present, unknown


def main():
    """Execute the commands from the command line."""
    exitcode = 0
    args = get_arg_parser().parse_args()
    deps = get_dependencies(args.depfile)
    known = deps.union(args.ignore)
    present, unknown  = compare_dependencies(known, args.srcdir)
    missing = deps.difference(present)
    if missing:
        print("Given dependencies missing:\n {}".format("\n ".join(missing)))
        exitcode = 1
    if unknown:
        print("Extant directories unknown:\n {}".format("\n ".join(unknown)))
        exitcode = 1
    return exitcode


def get_arg_parser():
    """Return the argument parser for this program."""
    parser = argparse.ArgumentParser("Compare dependencies with src tree")
    parser.add_argument("-i", "--ignore", action="append",
        help="The dependencies.tsv file to check")
    parser.add_argument("depfile", help="The dependencies.tsv file to check")
    parser.add_argument("srcdir", help="The go src dir to compare")
    return parser


if __name__ == '__main__':
    sys.exit(main())
