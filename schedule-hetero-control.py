#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
import os

from jenkins import Jenkins
from utility import (
    find_candidates,
    get_auth_token,
    get_candidates_path,
    )


def get_args():
    parser = ArgumentParser()
    parser.add_argument(
        'root_dir', help='Directory containing releases and candidates dir')
    return parser.parse_args()


def get_releases(root):
    release_path = os.path.join(root, 'old-juju')
    for entry in os.listdir(release_path):
        if not os.path.isdir(os.path.join(release_path, entry)):
            continue
        yield entry


def calculate_jobs(root):
    releases = list(get_releases(root))
    candidates_path = get_candidates_path(root)
    for candidate_path in find_candidates(root):
        parent, candidate = os.path.split(candidate_path)
        if parent != candidates_path:
            raise ValueError('Wrong path')
        for release in releases:
            if release == candidate:
                continue
            yield {
                'old_version': release,
                'candidate': candidate,
                'new_to_old': 'true'
            }
            yield {
                'old_version': release,
                'candidate': candidate,
                'new_to_old': 'false'
            }


def build_jobs(root, jobs):
    jenkins = Jenkins('http://localhost:8080')
    token = get_auth_token(root, 'compatibility-control')
    for job in jobs:
        jenkins.build_job('compatibility-control', job, token=token)


def main():
    args = get_args()
    build_jobs(args.root_dir, calculate_jobs(args.root_dir))


if __name__ == '__main__':
    main()
