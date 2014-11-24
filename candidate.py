#!/usr/bin/python
"""Manage the success juju revision testing candiates."""

from __future__ import print_function

from argparse import ArgumentParser
import os
import shutil
import sys
import traceback

from jujuci import (
    get_build_data,
    get_artifacts,
    JENKINS_URL,
)


BUILD_REVISION = 'build-revision'
PUBLISH_REVISION = 'publish-revision'


def find_publish_revision_number(br_number, limit=20):
    """Return the publish-revsion number paired with build-revision number."""
    description = 'Revision build: %s' % br_number
    found_number = None
    job_number = 'lastSuccessfulBuild'
    for i in range(limit):
        build_data = get_build_data(
            JENKINS_URL, PUBLISH_REVISION, build=job_number)
        if not build_data:
            return None
        # Ensure we have the real job number (an int), not an alias.
        job_number = build_data['number']
        if build_data['description'] == description:
            found_number = job_number
            break
        job_number = job_number - 1
    return found_number


def update_candate(branch, path, br_number,
                   pr_number=None, dry_run=False, verbose=False):
    """Download the files from the build-revision and publish-revision jobs.

    The buildvars.json for the specific build-revision number is downloaded.
    All the binary and source packages from the last successful build of
    publish revision are downloaded.
    """
    branch_name = branch.split(':')[1]
    artifact_dir_name = '%s-artifacts' % branch_name
    candidate_dir = os.path.join(path, artifact_dir_name)
    if os.path.isdir(candidate_dir):
        if verbose:
            print('Cleaning %s' % candidate_dir)
        if not dry_run:
            shutil.rmtree(candidate_dir)
    else:
        if verbose:
            print('Creating %s' % candidate_dir)
    if not dry_run:
        os.makedirs(candidate_dir)
    get_artifacts(
        BUILD_REVISION, br_number, 'buildvars.json', candidate_dir,
        dry_run=dry_run, verbose=verbose)
    if not pr_number:
        pr_number = find_publish_revision_number(br_number)
    get_artifacts(
        PUBLISH_REVISION, pr_number, 'juju-core*', candidate_dir,
        dry_run=dry_run, verbose=verbose)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Manage the successful Juju CI candidates.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-b', '--br-number', default='lastSuccessfulBuild',
        help="The specific build-revision number.")
    parser.add_argument(
        '-p', '--pr-number',
        help="The specific publish-revision-revision number.")
    parser.add_argument(
        'command', choices=['update'], help='The action to perform.')
    parser.add_argument(
        'branch', help='The successfully test branch location.')
    parser.add_argument(
        'path', help='The path to save the candiate data to.')
    return parser.parse_args(args)


def main(argv):
    """Manage successful Juju CI candiates."""
    args = parse_args(argv)
    try:
        if args.command == 'update':
            update_candate(
                args.branch, args.path, args.br_number, args.pr_number,
                dry_run=args.dry_run, verbose=args.verbose)
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
