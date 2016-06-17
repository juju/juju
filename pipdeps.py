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


BUCKET = "juju-pip-archives"
PREFIX = "juju-ci-tools/"
REQUIREMENTS = os.path.join(os.path.realpath(os.path.dirname(__file__)),
                            "requirements.txt")


def s3_anon():
    """Gives an unauthenticated S3 connection."""
    return boto.s3.connection.S3Connection(anon=True)


def s3_auth_with_rc(cloud_city):
    """Gives authenticated S3 connection using cloud-city credentials."""
    access_key = secret_key = None
    with open(os.path.join(cloud_city, "ec2rc")) as rc:
        for line in rc:
            parts = line.rstrip().split("=", 1)
            if parts[0] == "AWS_ACCESS_KEY":
                access_key = parts[1]
            elif parts[0] == "AWS_SECRET_KEY":
                secret_key = parts[1]
    return boto.s3.connection.S3Connection(access_key, secret_key)


def run_pip_install(extra_args, verbose=False):
    """Run pip install in a subprocess with given additional arguments."""
    cmd = ["pip"]
    if not verbose:
        cmd.append("-q")
    cmd.extend(["install", "-r", REQUIREMENTS])
    cmd.extend(extra_args)
    subprocess.check_call(cmd)


def command_install(bucket, verbose=False):
    with utility.temp_dir() as archives_dir:
        for key in bucket.list(prefix=PREFIX):
            archive = key.name[len(PREFIX):]
            key.get_contents_to_filename(os.path.join(archives_dir, archive))
        archives_url = "file://" + archives_dir
        run_pip_install(["--user", "--no-index", "--find-links", archives_url],
                        verbose=verbose)


def command_update(s3, verbose=False):
    bucket = s3.lookup(BUCKET)
    if bucket is None:
        if verbose:
            print("Creating bucket {}".format(BUCKET))
        bucket = s3.create_bucket(BUCKET, policy="public-read")
    with utility.temp_dir() as archives_dir:
        run_pip_install(["--download", archives_dir], verbose=verbose)
        for archive in os.listdir(archives_dir):
            filename = os.path.join(archives_dir, archive)
            key = boto.s3.key.Key(bucket)
            key.key = PREFIX + archive
            key.set_contents_from_filename(filename, policy="public-read")


def command_list(bucket, verbose=False):
    for key in bucket.list(prefix=PREFIX):
        print(key.name[len(PREFIX):])


def command_delete(bucket, verbose=False):
    for key in bucket.list(prefix=PREFIX):
        if verbose:
            print("Deleting {}".format(key.name))
        key.delete()


def get_parser(argv0):
    """Return parser for program arguments."""
    parser = argparse.ArgumentParser(
        prog=argv0, description="Manage pip dependencies")
    parser.add_argument(
        "-v", "--verbose", action="store_true", help="Show more output.")
    parser.add_argument(
        "--cloud-city", default="~/cloud-city", type=os.path.expanduser,
        help="Location of cloud-city repository for credentials.")
    parser.add_argument(
        "--requirements", default=REQUIREMENTS, type=os.path.expanduser,
        help="Location requirements file to use.")
    subparsers = parser.add_subparsers(dest="command")
    subparsers.add_parser("install", help="Download deps from S3 and install.")
    subparsers.add_parser(
        "update", help="Get latest deps from PyPI and upload to S3.")
    subparsers.add_parser("list", help="Show packages currently in S3.")
    subparsers.add_parser("delete", help="Delete packages currently in S3.")
    return parser


def main(argv):
    parser = get_parser(argv[0])
    args = parser.parse_args(argv[1:])
    use_auth = os.path.isdir(args.cloud_city)
    if not use_auth and args.command in ("update", "delete"):
        parser.error("Need cloud-city credentials to modify S3 cache.")
    s3 = s3_auth_with_rc(args.cloud_city) if use_auth else s3_anon()
    if args.command == "update":
        command_update(s3, args.verbose)
    else:
        bucket = s3.get_bucket(BUCKET)
        if args.command == "install":
            command_install(bucket, args.verbose)
        elif args.command == "list":
            command_list(bucket, args.verbose)
        elif args.command == "delete":
            command_delete(bucket, args.verbose)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
