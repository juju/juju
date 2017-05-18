#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
import json
import os
import re

from jenkins import Jenkins
from jujuci import (
    add_credential_args,
    get_credentials,
    )
from utility import (
    find_candidates,
    get_candidates_path,
    )


def get_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument(
        'root_dir', help='Directory containing releases and candidates dir')
    parser.add_argument(
        '--all', action='store_true', default=False,
        help='Schedule all candidates for client-server testing.')
    add_credential_args(parser)
    args = parser.parse_args(argv)
    return args, get_credentials(args)


def get_releases(root):
    release_path = os.path.join(root, 'old-juju')
    released_pattern = re.compile('^\d+\.\d+\.\d+[^~]*$')
    for entry in os.listdir(release_path):
        if not os.path.isdir(os.path.join(release_path, entry)):
            continue
        if released_pattern.match(entry):
            yield entry


def get_candidate_info(candidate_path):
    """ Return candidate version and revision build number. """
    with open(os.path.join(candidate_path, 'buildvars.json')) as fp:
        build_vars = json.load(fp)
    return build_vars['version'], build_vars['revision_build']


def calculate_jobs(root, schedule_all=False):
    releases = list(get_releases(root))
    candidates_path = get_candidates_path(root)
    for candidate_path in find_candidates(root, schedule_all):
        parent, candidate = os.path.split(candidate_path)
        if candidate.startswith('1.26'):
            # 1.26 was renamed to 2.0 because it is not compatible with 1.x
            continue
        if parent != candidates_path:
            raise ValueError('Wrong path')
        candidate_version, revision_build = get_candidate_info(candidate_path)
        for release in releases:
            # Releases with the same major number must be compatible.
            if release[:2] != candidate[:2]:
                continue
            for client_os in ('ubuntu', 'osx', 'windows'):
                yield {
                    'old_version': release,  # Client
                    'candidate': candidate_version,  # Server
                    'new_to_old': 'true',
                    'candidate_path': candidate,
                    'client_os': client_os,
                    'revision_build': revision_build,
                }
                yield {
                    'old_version': release,  # Server
                    'candidate': candidate_version,  # Client
                    'new_to_old': 'false',
                    'candidate_path': candidate,
                    'client_os': client_os,
                    'revision_build': revision_build,
                }


def build_jobs(credentials, root, jobs):
    jenkins = Jenkins('http://juju-ci.vapour.ws:8080', *credentials)
    os_str = {"ubuntu": "", "osx": "-osx", "windows": "-windows"}
    for job in jobs:
        jenkins.build_job(
            'compatibility-control{}'.format(os_str[job['client_os']]), job)


def main():
    args, credentials = get_args()
    build_jobs(
        credentials, args.root_dir, calculate_jobs(args.root_dir, args.all))


if __name__ == '__main__':
    main()
