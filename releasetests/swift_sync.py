#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import hashlib
import json
import os
import subprocess
import sys
import urllib2


MAX_UPLOAD_ATTEMPTS = 3


def get_account():
    cmd = ['swift', 'stat']
    output = subprocess.check_output(cmd)
    account = None
    for line in output.split('\n'):
        if 'Account:' in line:
            account = line.split(':')[1].strip()
    return account


def get_files(args):
    swift_url = os.environ['OS_SWIFT_URL']
    account = get_account()
    container_url = '{0}/{1}/{2}?prefix={3}&format=json'.format(
        swift_url, account, args.container, args.path)
    if args.verbose:
        print("Checking {0}".format(container_url))
    response = urllib2.urlopen(container_url)
    files = json.loads(response.read())
    remote_files = dict((f['name'], f) for f in files)
    return remote_files


def upload_changes(args, remote_files):
    container_path = os.path.join(args.container, args.path)
    count = 0
    uploaded_files = []
    for file_name in args.files:
        local_path = os.path.join(args.path, file_name)
        remote_file = remote_files.get(local_path)
        if remote_file is None:
            if args.verbose:
                print("File is new: {0}".format(local_path))
        else:
            md5 = hashlib.md5()
            with open(file_name, mode='rb') as local_file:
                md5.update(local_file.read())
            remote_hash = str(remote_file['hash'])
            local_hash = str(md5.hexdigest())
            if remote_hash == local_hash:
                if args.verbose:
                    print("File is same: {0}".format(local_path))
                continue
            else:
                if args.verbose:
                    print("File is different: {0}".format(local_path))
                    print("  {0} != {1}".format(local_hash, remote_hash))
        count += 1
        print("Uploading {0}/{1}".format(args.container, local_path))
        cmd = ['swift', 'upload', container_path, file_name]
        if not args.dry_run:
            uploaded = False
            attempt = 1
            while not uploaded and attempt <= MAX_UPLOAD_ATTEMPTS:
                # python-swiftclient on precise doesn't support --retry.
                try:
                    output = subprocess.check_output(cmd)
                    uploaded = True
                except subprocess.CalledProcessError:
                    attempt += 1
                    if attempt > MAX_UPLOAD_ATTEMPTS:
                        raise
            print(' '.join(cmd))
            print(output)
            uploaded_files.append(output)
    print('Uploaded {0} files'.format(count))
    return uploaded_files


def main():
    parser = ArgumentParser('Sync changed and new files.')
    parser.add_argument(
        "-d", "--dry-run", action="store_true", default=False,
        help="Do not make changes")
    parser.add_argument(
        '-v', '--verbose', action="store_true", help='Increse verbosity.')
    parser.add_argument(
        '--container', default='juju-dist', help='The container name.')
    parser.add_argument('path', help='The destination path in the container.')
    parser.add_argument(
        'files', nargs='*', help='The files to send to the container.')
    args = parser.parse_args()
    if not os.environ.get('OS_AUTH_URL'):
        print('OS_AUTH_URL must be sourced into the environment.')
        sys.exit(1)
    remote_files = get_files(args)
    upload_changes(args, remote_files)

if __name__ == '__main__':
    main()
