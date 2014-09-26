#!/usr/bin/env python

from __future__ import print_function

from argparse import ArgumentParser
import errno
import os.path
import urllib
import shutil
from subprocess import (
    check_call,
    check_output,
)


def get_args():
    parser = ArgumentParser()
    parser.add_argument('version', help='The version number of this juju.')
    parser.add_argument('branch', help='The branch this juju came from.')
    parser.add_argument('jenkins_url',
                        help='URL to the jenkins with binaries.')
    parser.add_argument('target_dir', help='Directory to extract to.')
    return parser.parse_args()


def extract_binary(version, branch, jenkins_url, target_dir):
    if jenkins_url.endswith('/'):
        jenkins_url = jenkins_url[0:-2]
    real_target_dir = os.path.realpath(target_dir)
    if branch == 'gitbranch:master:github.com/juju/juju':
        full_target = os.path.join(real_target_dir, 'master')
    else:
        full_target = os.path.join(real_target_dir, 'stable')
    release = check_output(['lsb_release', '-sr']).strip()
    arch = check_output(['dpkg', '--print-architecture']).strip()
    juju_core_deb = 'juju-core_{}-0ubuntu1~{}.1~juju1_{}.deb'.format(
        version, release, arch)
    encoded_core_deb = urllib.quote(juju_core_deb)
    deb_url = '{}/job/publish-revision/lastSuccessfulBuild/artifact/{}'.format(
        jenkins_url, encoded_core_deb)
    try:
        os.unlink(juju_core_deb)
    except OSError as e:
        if e.errno != errno.ENOENT:
            raise
    check_call(['wget', '-q', '-O', juju_core_deb, deb_url])
    shutil.rmtree(full_target)
    check_call(['dpkg', '-x', juju_core_deb, full_target])
    print("Extracted juju to {}".format(full_target))


def main():
    args = get_args()
    extract_binary(args.version, args.branch, args.jenkins_url,
                   args.target_dir)


if __name__ == '__main__':
    main()
