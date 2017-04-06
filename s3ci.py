#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
from ConfigParser import ConfigParser
import json
import logging
import os
import re
import sys

from boto.s3.connection import S3Connection

from download_juju import (
    download_files,
    filter_keys,
    )
from jujuci import (
    acquire_binary,
    JobNamer,
    PackageNamer,
    )
from jujupy import get_juju_home
from utility import configure_logging

JUJU_QA_DATA = 'juju-qa-data'


def parse_args(args=None):
    parser = ArgumentParser()
    default_config = os.path.join(get_juju_home(), 'juju-qa.s3cfg')
    subparsers = parser.add_subparsers(help='Sub-command to run',
                                       dest='command')
    parser_get_juju_bin = subparsers.add_parser(
        'get-juju-bin', help='Retrieve and extract juju binaries.',
        description="""
        Download the package for this client machine from s3.
        Extract the package files.  Print the location of the juju client.
        """)
    parser_get = subparsers.add_parser(
        'get', help='Download job files(s)',
        description="""
        Download a file from a job for this revision build.

        Within the revision build, the most recent version will be selected.
        """)
    parser_get_summary = subparsers.add_parser(
        'get-summary', help='Summarize this test',
        description="""
        Print a summary of what is being tested.
        """
        )
    for subparser in [parser_get_juju_bin, parser_get, parser_get_summary]:
        subparser.add_argument(
            'revision_build', type=int,
            help='The revision-build to use.')
    parser_get.add_argument('job', help='The job to get files from',)
    parser_get.add_argument(
        'file_pattern', help='The file pattern to use for selecting files',)
    parser_get_summary.add_argument(
        'env', help='The name of the environment used for this test.')
    for subparser in [parser_get_juju_bin, parser_get]:
        subparser.add_argument(
            'workspace', nargs='?', default='.',
            help='The directory to download into')
    for subparser in [parser_get_juju_bin, parser_get, parser_get_summary]:
        subparser.add_argument(
            '--config', default=default_config,
            help=('s3cmd config file for credentials.  Default to '
                  'juju-qa.s3cfg in juju home.'))
        subparser.add_argument(
            '--verbose', '-v', default=0, action='count',
            help='Increase verbosity')
    return parser.parse_args(args)


def get_s3_credentials(s3cfg_path):
    config = ConfigParser()
    with open(s3cfg_path) as fp:
        config.readfp(fp)
    access_key = config.get('default', 'access_key')
    secret_key = config.get('default', 'secret_key')
    return access_key, secret_key


def get_job_path(revision_build, job):
    return 'juju-ci/products/version-{}/{}'.format(revision_build, job)


class PackageNotFound(Exception):
    """Raised when a package cannot be found."""


def find_package_key(bucket, revision_build):
    namer = JobNamer.factory()
    job = namer.get_build_binary_job()
    prefix = get_job_path(revision_build, job)
    keys = bucket.list(prefix)
    suffix = PackageNamer.factory().get_release_package_suffix()
    filtered = [
        (k, f) for k, f in filter_keys(keys, suffix)
        if f.startswith('juju-core_')]
    if len(filtered) == 0:
        raise PackageNotFound('Package could not be found.')
    return sorted(filtered, key=lambda x: int(
        re.search(r'build-(\d+)/', x[0].name).group(1)))[-1]


def fetch_juju_binary(bucket, revision_build, workspace):
    package_key, filename = find_package_key(bucket, revision_build)
    logging.info('Selected: %s', package_key.name)
    download_files([package_key], workspace)
    package_path = os.path.join(workspace, filename)
    logging.info('Extracting: %s', package_path)
    return acquire_binary(package_path, workspace)


def find_file_keys(bucket, revision_build, job, file_regex):
    prefix = get_job_path(revision_build, job)
    keys = bucket.list(prefix)
    by_build = {}
    for key in keys:
        logging.debug('looking at {}'.format(key.name))
        match = re.search('^/build-(\d+)', key.name[len(prefix):])
        if match is None:
            logging.debug('job name is not /{}/'.format(job))
            continue
        build = int(match.group(1))
        by_build.setdefault(build, []).append(key)
    # We can't use last successful build, because we don't know what builds
    # are successful, so use last build and require it to be successful.
    last_build = max(by_build.keys())
    build_keys = by_build[last_build]
    filtered = []
    full_prefix = '{}/build-{}/'.format(prefix, last_build)
    for key in build_keys:
        path = key.name[len(full_prefix):]
        if re.match(file_regex, path):
            filtered.append(key)
    return filtered


def fetch_files(bucket, revision_build, job, file_pattern, workspace):
    file_keys = find_file_keys(bucket, revision_build, job, file_pattern)
    out_files = [os.path.join(workspace, k.name.split('/')[-1])
                 for k in file_keys]
    for key in file_keys:
        logging.info('Selected: %s', key.name)
    download_files(file_keys, workspace)
    return out_files


def main():
    args = parse_args()
    log_level = logging.WARNING - args.verbose * (
        logging.WARNING - logging.INFO)
    configure_logging(log_level)
    if args.command == 'get-juju-bin':
        return get_juju_bin(args)
    elif args.command == 'get':
        return cmd_get(args)
    elif args.command == 'get-summary':
        return cmd_get_summary(args)
    else:
        raise Exception('{} not implemented.'.format(args.command))


def get_qa_data_bucket(config):
    credentials = get_s3_credentials(config)
    conn = S3Connection(*credentials)
    return conn.get_bucket(JUJU_QA_DATA)


def get_juju_bin(args):
    bucket = get_qa_data_bucket(args.config)
    print(fetch_juju_binary(bucket, args.revision_build, args.workspace))


def cmd_get(args):
    bucket = get_qa_data_bucket(args.config)
    for path in fetch_files(bucket, args.revision_build, args.job,
                            args.file_pattern, args.workspace):
        print(path)


def get_summary(bucket, revision_build, env):
    """Return a textual summary of a test.

    revision-build is the build number of the build-revision that is under
    test.

    env is the name of the environmnet being used to test.
    """
    template = (
        'Testing {branch} {short_revision_id} on {env} for {revision_build}')
    job_path = get_job_path(revision_build, 'build-revision')
    buildvars_path = 'build-{}/buildvars.json'.format(revision_build)
    full_path = '/'.join([job_path, buildvars_path])
    key = bucket.get_key(full_path)
    buildvars = json.loads(key.get_contents_as_string())
    short_revision_id = buildvars['revision_id'][0:7]
    return template.format(
        short_revision_id=short_revision_id, env=env, **buildvars)


def cmd_get_summary(args):
    """Print a textual summary of a test."""
    bucket = get_qa_data_bucket(args.config)
    print(get_summary(bucket, args.revision_build, args.env))


if __name__ == '__main__':
    sys.exit(main())
