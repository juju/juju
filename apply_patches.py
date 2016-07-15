#!/usr/bin/python
"""Script for applying a directory of patches to a source tree."""

from __future__ import print_function

from argparse import ArgumentParser
import os
import shutil
import subprocess
import sys


def apply_patch(patch_file, base_dir, dry_run=False, verbose=False):
    """Run external patch command to apply given patch_file to base_dir."""
    patch_cmd = ["patch", "-f", "-u", "-p1", "-r-"]
    if dry_run:
        patch_cmd.append("--dry-run")
    if verbose:
        patch_cmd.append("--verbose")
    with open(patch_file) as f:
        return subprocess.call(patch_cmd, cwd=base_dir, stdin=f)


def get_arg_parser():
    """Return the argument parser for this program."""
    parser = ArgumentParser("Apply patches to source tree")
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Do not actually modify source tree")
    parser.add_argument(
        "--verbose", action="store_true",
        help="Show more output while patching")
    parser.add_argument("patchdir", help="The dir containing patch files")
    parser.add_argument("srctree", help="The base source tree to modify")
    return parser


def main(argv):
    """Parse argv and run program logic."""
    parser = get_arg_parser()
    args = parser.parse_args(argv[1:])
    try:
        patches = sorted(os.listdir(args.patchdir))
    except OSError as e:
        parser.error("Could not list patchdir: {}".format(e))
    if not os.path.isdir(args.srctree):
        parser.error("Given srctree '{}' not a directory".format(args.srctree))
    print("Applying {} patches".format(len(patches)))
    for patch in patches:
        patch_path = os.path.join(args.patchdir, patch)
        if apply_patch(patch_path, args.srctree, args.dry_run, args.verbose):
            print("Failed to apply patch '{}'".format(patch))
            return 1
        print("Applied patch '{}'".format(patch))
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
