#!/usr/bin/env python2
from __future__ import print_function

from argparse import ArgumentParser
import os
import xml.etree.ElementTree as ET

from jenkins import Jenkins


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
    candidates = ['master', 'stable']
    for candidate in candidates:
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
    tree = ET.parse(os.path.join(root,
                                 'jobs/compatibility-control/config.xml'))
    token = tree.getroot().find('authToken').text
    for job in jobs:
        jenkins.build_job('compatibility-control', job, token=token)


def main():
    args = get_args()
    build_jobs(args.root_dir, calculate_jobs(args.root_dir))


if __name__ == '__main__':
    main()
