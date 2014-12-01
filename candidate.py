#!/usr/bin/python
"""Manage the success juju revision testing candiates."""

from __future__ import print_function

from argparse import ArgumentParser
import json
import os
import shutil
import subprocess
import sys
import traceback

from jujuci import (
    get_build_data,
    get_artifacts,
    JENKINS_URL,
)
from utility import temp_dir


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


def prepare_dir(dir_path, dry_run=False, verbose=False):
    if os.path.isdir(dir_path):
        if verbose:
            print('Cleaning %s' % dir_path)
        if not dry_run:
            shutil.rmtree(dir_path)
    else:
        if verbose:
            print('Creating %s' % dir_path)
    if not dry_run:
        os.makedirs(dir_path)


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
    prepare_dir(candidate_dir, dry_run, verbose)
    get_artifacts(
        BUILD_REVISION, br_number, 'buildvars.json', candidate_dir,
        dry_run=dry_run, verbose=verbose)
    if not pr_number:
        pr_number = find_publish_revision_number(br_number)
    get_artifacts(
        PUBLISH_REVISION, pr_number, 'juju-core*', candidate_dir,
        dry_run=dry_run, verbose=verbose)


def get_artifact_dirs(path):
    dirs = []
    for name in os.listdir(path):
        artifacts_path = os.path.join(path, name)
        if name.endswith('-artifacts') and os.path.isdir(artifacts_path):
            dirs.append(name)
    return dirs


def get_package(artifacts_path, version):
    release = subprocess.check_output(['lsb_release', '-sr']).strip()
    arch = subprocess.check_output(['dpkg', '--print-architecture']).strip()
    package_name = 'juju-core_{}-0ubuntu1~{}.1~juju1_{}.deb'.format(
        version, release, arch)
    package_path = os.path.join(artifacts_path, package_name)
    return package_path


def extract_candidates(path, dry_run=False, verbose=False):
    for dir_name in get_artifact_dirs(path):
        artifacts_path = os.path.join(path, dir_name)
        buildvars_path = os.path.join(artifacts_path, 'buildvars.json')
        with open(buildvars_path) as bf:
            buildvars = json.load(bf)
        version = buildvars['version']
        package_path = get_package(artifacts_path, version)
        branch_name = dir_name.split('-')[0]
        candidate_path = os.path.join(path, branch_name)
        if verbose:
            print('extracting %s to %s' % (package_path, candidate_path))
        prepare_dir(candidate_path, dry_run, verbose)
        command = ['dpkg', '-x', package_path, candidate_path]
        if not dry_run:
            subprocess.check_call(command)
        if verbose:
            print('Copying %s to %s' % (buildvars_path, candidate_path))
        if not dry_run:
            shutil.copyfile(buildvars_path, candidate_path)


def get_scripts(juju_release_tools=None):
    assemble_script = 'assemble-streams.bash'
    publish_script = 'publish-public-tools.bash'
    if juju_release_tools:
        assemble_script = os.path.join(
            juju_release_tools, assemble_script)
        publish_script = os.path.join(
            juju_release_tools, publish_script)
    return assemble_script, publish_script


def run_command(command, dry_run=False, verbose=False):
    if verbose:
        print('Executing: %s' % command)
    if not dry_run:
        output = subprocess.check_output(command)
        if verbose:
            print(output)


def publish_candidates(path, streams_path,
                       juju_release_tools=None, dry_run=False, verbose=False):
        with temp_dir() as debs_path:
            for dir_name in get_artifact_dirs(path):
                artifacts_path = os.path.join(path, dir_name)
                for deb_name in os.listdir(artifacts_path):
                    deb_path = os.path.join(artifacts_path, deb_name)
                    print('Copying %s' % deb_path)
                    new_path = os.path.join(debs_path, deb_name)
                    shutil.copyfile(deb_path, new_path)
            assemble_script, publish_script = get_scripts(juju_release_tools)
            # XXX sinzui 2014-12-01: IGNORE uses the local juju, but when
            # testing juju's that change generate-tools, we may need to use
            # the highest version.
            command = [
                assemble_script, '-t',  debs_path, 'weekly', 'IGNORE',
                streams_path]
            run_command(command, dry_run=dry_run, verbose=verbose)
        juju_dist_path = os.path.join(streams_path, 'juju-dist')
        command = [publish_script,  'weekly', juju_dist_path, 'cpc']
        run_command(command, dry_run=dry_run, verbose=verbose)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Manage the successful Juju CI candidates.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    # ./candidate update -b 1234 master ~/candidate
    parser_update = subparsers.add_parser('update', help='Update candidate')
    parser_update.add_argument(
        '-b', '--br-number', default='lastSuccessfulBuild',
        help="The specific build-revision number.")
    parser_update.add_argument(
        '-p', '--pr-number',
        help="The specific publish-revision-revision number.")
    parser_update.add_argument(
        'branch', help='The successfully test branch location.')
    parser_update.add_argument(
        'path', help='The path to save the candiate data to.')
    # ./candidate extract master ~/candidate
    parser_extract = subparsers.add_parser(
        'extract',
        help='extract candidates that match the local series and arch.')
    parser_extract.add_argument(
        'path', help='The path to the candiate data dir.')
    # ./candidate publish ~/candidate
    parser_publish = subparsers.add_parser(
        'publish', help='Publish streams for the candidates')
    parser_publish.add_argument(
        '-t', '--juju-release-tools',
        help='The path to the juju-release-tools dir.')
    parser_publish.add_argument(
        'path', help='The path to the candiate data dir.')
    parser_publish.add_argument(
        'streams_path', help='The path to the streams data dir.')
    return parser.parse_args(args)


def main(argv):
    """Manage successful Juju CI candiates."""
    args = parse_args(argv)
    try:
        if args.command == 'update':
            update_candate(
                args.branch, args.path, args.br_number, args.pr_number,
                dry_run=args.dry_run, verbose=args.verbose)
        elif args.command == 'extract':
            extract_candidates(
                args.path, dry_run=args.dry_run, verbose=args.verbose)
        elif args.command == 'publish':
            publish_candidates(
                args.path, args.streams_path,
                juju_release_tools=args.juju_release_tools,
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
