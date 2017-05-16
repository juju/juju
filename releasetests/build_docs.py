#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import os
import shutil
import subprocess
import sys
import tempfile


DEFAULT_SOURCE_URL = 'https://github.com/juju/docs.git'
DEFAULT_DESTINATION_URL = (
    'https://code.launchpad.net/~charmers/juju-core/github-docs')


def build_docs(cwd, args):
    local_upstream = os.path.join(args.local, 'upstream')
    local_downstream = os.path.join(args.local, 'downstream')
    output = subprocess.check_output(
        ['bzr', 'branch', args.destination, local_downstream])
    if args.verbose:
        print(output)
    output = subprocess.check_output(
        ['git', 'clone', args.source, local_upstream])
    if args.verbose:
        print(output)
    os.chdir(local_upstream)
    output = subprocess.check_output(['git', 'checkout', args.branch])
    if args.verbose:
        print(output)
    message = subprocess.check_output(
        ['git', 'log', '--format=oneline', '-1'])
    output = subprocess.check_output(['grep', 'apt-get install', 'Makefile'])
    if args.verbose:
        print('These packages are requried to build docs.')
        print(output)
    output = subprocess.check_output(['make', 'build'])
    if args.verbose:
        print(output)
    for child in os.listdir(local_downstream):
        # Remove the directories to ensure files from previous revsions
        # are not left behind.
        downstream_path = os.path.join(local_downstream, child)
        if os.path.isdir(downstream_path) and not child.startswith('.'):
            shutil.rmtree(downstream_path)
        elif os.path.isfile(downstream_path):
            os.remove(downstream_path)
    ignorable = shutil.ignore_patterns(('*.pyc', '^.git', '^.bzr'))
    for child in os.listdir(local_upstream):
        # Remove the directories to ensure files from previous revsions
        # are not left behind.
        upstream_path = os.path.join(local_upstream, child)
        if os.path.isdir(upstream_path) and not child.startswith('.'):
            shutil.copytree(
                upstream_path, os.path.join(local_downstream, child),
                ignore=ignorable)
        elif os.path.isfile(upstream_path):
            shutil.copy(upstream_path, local_downstream)
    os.chdir(local_downstream)
    output = subprocess.check_output(['bzr', 'add'])
    output = subprocess.check_output(['bzr', 'commit', '-m', message])
    if args.verbose:
        print(output)
    # push local bzr to destination.


def main(cwd):
    parser = ArgumentParser('Build and republish Juju docs.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', help='Increase verbosity.')
    parser.add_argument(
        '-s', '--source', default=DEFAULT_SOURCE_URL,
        help='The source repo/branch of docs. Default={}'.format(
            DEFAULT_SOURCE_URL))
    parser.add_argument(
        '-b', '--branch', default='master',
        help='The branch to checkout from the source repository')
    parser.add_argument(
        '-d', '--destination', default=DEFAULT_DESTINATION_URL,
        help='The destination repo/branch of docs. Default={}'.format(
            DEFAULT_DESTINATION_URL))
    parser.add_argument(
        '-l', '--local',
        help='The local directory to place the unbuilt and built docs.')
    args = parser.parse_args()
    if args.local is None:
        args.local = tempfile.mkdtemp(prefix='docs.', dir=cwd)
    else:
        if not os.path.isdir(args.local):
            sys.exit(1)
    try:
        build_docs(cwd, args)
    except Exception as err:
        print(str(err))


if __name__ == '__main__':
    cwd = os.getcwd()
    main(cwd)
