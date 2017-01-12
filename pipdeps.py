#!/usr/bin/env python
"""Manage pip dependencies for juju qa using a cache in S3."""

from __future__ import print_function

import argparse
from distutils.version import LooseVersion
import os
import platform
import subprocess
import sys

import boto.s3.connection
import boto.s3.key

import utility


BUCKET = "juju-pip-archives"
PREFIX = "juju-ci-tools/"
REQUIREMENTS = os.path.join(os.path.realpath(os.path.dirname(__file__)),
                            "requirements.txt")
REQUIREMENTS_PY3 = os.path.join(os.path.realpath(os.path.dirname(__file__)),
                                "requirements_py3.txt")
MAC_WIN_REQS = os.path.join(os.path.realpath(os.path.dirname(__file__)),
                            "mac-win-requirements.txt")
OBSOLETE = os.path.join(os.path.realpath(os.path.dirname(__file__)),
                        "obsolete-requirements.txt")


def get_requirements(python3=False):
    """Return requirements file path."""
    if platform.dist()[0] in ('Ubuntu', 'debian'):
        if python3:
            return REQUIREMENTS_PY3
        return REQUIREMENTS
    else:
        if python3:
            return None
        return MAC_WIN_REQS


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


def is_py3_supported():
    """Determine if Python3 and pip3 are installed."""
    try:
        version = subprocess.check_output(
            ['python3', '--version'], stderr=subprocess.STDOUT)
    except OSError:
        return False
    version = version.strip().split()[-1]
    # Python on yakkety reports its version as "3.5.2+", so use LooseVersion.
    if LooseVersion(version) < LooseVersion('3.5'):
        return False
    try:
        subprocess.check_call(['pip3', '--version'])
    except OSError:
        return False
    return True


def run_pip3_install(extra_args, requirements, verbose=False):
    if requirements and is_py3_supported():
        run_pip_install(extra_args, requirements, verbose=verbose,
                        cmd=['pip3'])
    else:
        print('Python 3 is not installed.')


def run_pip_install(extra_args, requirements, verbose=False, cmd=None):
    """Run pip install in a subprocess with given additional arguments."""
    cmd = cmd or ['pip']
    if not verbose:
        cmd.append("-q")
    cmd.extend(["install", "-r", requirements])
    cmd.extend(extra_args)
    subprocess.check_call(cmd)


def run_pip_uninstall(obsolete_requirements):
    """Run pip uninstall for each package version in obsolete_requirements.

    pip uninstall the package without regard to its version. In most cases,
    calling install with a new package version implicitly upgrades.
    There are only a few package version that cannot by upgraded, they must
    be removed before install. This function uninstalls packages only when
    their version matches the obsolete.

    The obsolete_requirements entries must match the output of pip list. eg:
        azure (0.8.0)
        bibbel (1.2.3)
    """
    pip_cmd = ['pip']
    list_cmd = pip_cmd + ['list']
    installed_packages = set(subprocess.check_output(list_cmd).splitlines())
    with open(obsolete_requirements, 'r') as o_file:
        obsolete_packages = o_file.read().splitlines()
    removable = installed_packages.intersection(obsolete_packages)
    for package_version in removable:
        package, version = package_version.split()
        uninstall_cmd = pip_cmd + ['uninstall', '-y', package]
        subprocess.check_call(uninstall_cmd)


def get_pip_args(archives_url):
    args = ["--no-index", "--find-links", archives_url]
    # --user option is invalid when running inside virtualenv. Check
    # sys.real_prefix to determine if it is executing inside virtualenv.
    # Inside virtualenv, the sys.prefix points to the virtualenv directory
    # and sys.real_prefix points to the real prefix.
    # Running outside a virtualenv, sys should not have real_prefix attribute.
    if not hasattr(sys, 'real_prefix'):
        args.append("--user")
    return args


def command_install(bucket, requirements, verbose=False,
                    requirements_py3=None):
    with utility.temp_dir() as archives_dir:
        for key in bucket.list(prefix=PREFIX):
            archive = key.name[len(PREFIX):]
            key.get_contents_to_filename(os.path.join(archives_dir, archive))
        archives_url = "file://" + archives_dir
        pip_args = get_pip_args(archives_url)
        run_pip_uninstall(OBSOLETE)
        run_pip_install(pip_args, requirements, verbose=verbose)
        run_pip3_install(pip_args, requirements_py3, verbose=verbose)


def command_update(s3, requirements, verbose=False):
    bucket = s3.lookup(BUCKET)
    if bucket is None:
        if verbose:
            print("Creating bucket {}".format(BUCKET))
        bucket = s3.create_bucket(BUCKET, policy="public-read")
    with utility.temp_dir() as archives_dir:
        run_pip_install(
            ["--download", archives_dir], requirements, verbose=verbose)
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
        "--requirements", default=get_requirements(), type=os.path.expanduser,
        help="Location requirements file to use.")
    parser.add_argument(
        "--requirements_py3", default=get_requirements(python3=True),
        type=os.path.expanduser,
        help="Location requirements file to use for Python 3.")
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
        command_update(s3, args.requirements, args.verbose)
    else:
        bucket = s3.get_bucket(BUCKET)
        if args.command == "install":
            command_install(
                bucket, args.requirements, args.verbose, args.requirements_py3)
        elif args.command == "list":
            command_list(bucket, args.verbose)
        elif args.command == "delete":
            command_delete(bucket, args.verbose)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
