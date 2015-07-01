#!/usr/bin/env python
"""Manage pip dependencies for juju qa using a cache in S3."""

from __future__ import print_function

import argparse
import os
import subprocess
import sys

import boto.s3.connection
import boto.s3.key

import utility


BUCKET = "juju-qa-data"
PREFIX = "pip-archives/"


def s3_from_rc(cloud_city):
    """Gives authenticated S3 connection using cloud-city credentials."""
    access_key = secret_key = None
    with open(os.path.join(os.path.expanduser(cloud_city), "ec2rc")) as rc:
        for line in rc:
            parts = line.rstrip().split("=", 1)
            if parts[0] == "AWS_ACCESS_KEY":
                access_key = parts[1]
            elif parts[0] == "AWS_SECRET_KEY":
                secret_key = parts[1]
    return boto.s3.connection.S3Connection(access_key, secret_key)


def run_pip_install(extra_args, verbose=False):
    """Run pip install in a subprocess with given additional arguments."""
    args = ["pip"]
    if not verbose:
        args.append("-q")
    requirements = os.path.join(os.path.dirname(__file__), "requirements.txt")
    args.extend(["install", "-r", requirements])
    args.extend(extra_args)
    subprocess.check_call(args)


def command_install(bucket, verbose=False):
    with utility.temp_dir() as archives_dir:
        for key in bucket.list(prefix=PREFIX):
            archive = key.name[len(PREFIX):]
            key.get_contents_to_filename(os.path.join(archives_dir, archive))
        run_pip_install(["--user", "--no-index", "--find-links", archives_dir],
            verbose=verbose)


def command_update(bucket, verbose=False):
    with utility.temp_dir() as archives_dir:
        run_pip_install(["--download", archives_dir], verbose=verbose)
        for archive in os.listdir(archives_dir):
            key = boto.s3.key.Key(bucket)
            key.key = PREFIX + archive
            key.set_contents_from_filename(os.path.join(archives_dir, archive))


def command_list(bucket, verbose=False):
    for key in bucket.list(prefix=PREFIX):
        print(key.name[len(PREFIX):])


def get_args(argv):
    """Parse and return arguments."""
    parser = argparse.ArgumentParser(
        prog=argv[0], description="Manage pip dependencies")
    parser.add_argument("-v", "--verbose", action="store_true",
        help="Show more output.")
    parser.add_argument("--cloud-city", default="~/cloud-city",
        help="Location of cloud-city repository for credentials.")
    subparsers = parser.add_subparsers(dest="command")
    subparsers.add_parser("install", help="Download deps from S3 and install.")
    subparsers.add_parser("update",
        help="Get latest deps from PyPI and upload to S3.")
    subparsers.add_parser("list", help="Show packages currently in S3.")
    return parser.parse_args(argv[1:])


def main(argv):
    args = get_args(argv)
    s3 = s3_from_rc(args.cloud_city)
    bucket = s3.get_bucket(BUCKET)
    if args.command == "install":
        # XXX: Should maybe not bother requiring S3 creds?
        command_install(bucket, args.verbose)
    elif args.command == "update":
        command_update(bucket, args.verbose)
    elif args.command == "list":
        command_list(bucket, args.verbose)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
