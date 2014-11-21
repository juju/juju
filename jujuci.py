#!/usr/bin/python
"""Access Juju CI artifacts and data."""

from __future__ import print_function

from argparse import ArgumentParser
from collections import namedtuple
import fnmatch
import json
import os
import shutil
import sys
import traceback
import urllib
import urllib2


JENKINS_URL = 'http://juju-ci.vapour.ws:8080'

Artifact = namedtuple('Artifact', ['file_name', 'location'])


def print_now(string):
    print(string)


def get_build_data(jenkins_url, job_name, build='lastSuccessfulBuild'):
    """Return a dict of the build data for a job build number."""
    build_data = urllib2.urlopen(
        '%s/job/%s/%s/api/json' % (jenkins_url, job_name, build))
    build_data = json.load(build_data)
    return build_data


def find_artifacts(build_data, glob='*'):
    found = []
    for artifact in build_data['artifacts']:
        file_name = artifact['fileName']
        if fnmatch.fnmatch(file_name, glob):
            location = '%sartifact/%s' % (build_data['url'], file_name)
            artifact = Artifact(file_name, location)
            found.append(artifact)
    return found


def download_files(files, path):
    for file_name in files:
        pass


def list_artifacts(job_name, build, glob, verbose=False):
    build_data = get_build_data(JENKINS_URL, job_name, build)
    artifacts = find_artifacts(build_data, glob)
    for artifact in artifacts:
        if verbose:
            print_now(artifact.location)
        else:
            print_now(artifact.file_name)


def get_artifacts(job_name, build, glob, path,
                  archive=False, dry_run=False, verbose=False):
    full_path = os.path.expanduser(path)
    if archive:
        if not os.path.isdir(full_path):
            raise ValueError('%s does not exist' % full_path)
        shutil.rmtree(full_path)
        os.makedirs(full_path)
    build_data = get_build_data(JENKINS_URL, job_name, build)
    artifacts = find_artifacts(build_data, glob)
    opener = urllib.URLopener()
    for artifact in artifacts:
        local_path = os.path.abspath(
            os.path.join(full_path, artifact.file_name))
        if verbose:
            print_now('Retrieving %s => %s' % (artifact.location, local_path))
        else:
            print_now(artifact.file_name)
        if not dry_run:
            opener.retrieve(artifact.location, local_path)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("List and get artifacts from Juju CI.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-a', '--archive', action='store_true', default=False,
        help='Ensure the download path exists and remove older files.')
    parser.add_argument(
        '-b', '--build', default='lastSuccessfulBuild',
        help="The specific build to examine (default: lastSuccessfulBuild).")
    parser.add_argument(
        'command', choices=['list', 'get'], help='The action to perform.')
    parser.add_argument(
        'job', help="The job that collected the artifacts.")
    parser.add_argument(
        'glob', nargs='?', default='*',
        help="The glob pattern to match artifact file names.")
    parser.add_argument(
        'path', nargs='?', default='.',
        help="The path to download the files to.")
    return parser.parse_args(args)


def main(argv):
    """Manage list and get files from jujuci builds."""
    args = parse_args(argv)
    try:
        if args.command == 'list':
            list_artifacts(
                args.job, args.build, args.glob, verbose=args.verbose)
        elif args.command == 'get':
            get_artifacts(
                args.job, args.build, args.glob, args.path,
                archive=args.archive, dry_run=args.dry_run,
                verbose=args.verbose)
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
