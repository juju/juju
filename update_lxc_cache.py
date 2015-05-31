#!/usr/bin/python
"""Build and package juju for an non-debian OS."""

from __future__ import print_function

from argparse import ArgumentParser
from collections import namedtuple
import os
import sys
import traceback
import requests
import shutil
import subprocess


SITE = 'https://images.linuxcontainers.org'
INDEX_PATH = 'meta/1.0'
INDEX = 'index-system'
ROOTFS = 'rootfs.tar.xz'
META = 'meta.tar.xz'
LXC_CACHE = '/var/cache/lxc/download'


System = namedtuple(
    'System', ['dist', 'release', 'arch', 'variant', 'version', 'path'])


PUT_SCRIPT = """\
scp {rootfs_path} {meta_path} {user_host}:~/
"""

INSTALL_SCRIPT = """\
ssh {user_host} bash <<"EOT"
sudo mkdir -p {lxc_cache}
sudo mv {rootfs} {meta} {lxc_cache}
sudo chown root:root  {lxc_cache}/*
sudo tar -C {lxc_cache} -xf {lxc_cache}/meta.tar.xz
EOT
"""


class LxcCache:
    """Manage the LXC download template cache."""

    def __init__(self, workspace, verbose=False, dry_run=False):
        self.workspace = os.path.abspath(workspace)
        self.verbose = verbose
        self.dry_run = dry_run
        local_path = os.path.join(self.workspace, INDEX_PATH, INDEX)
        self.systems = self.init_systems(local_path)

    def init_systems(self, location):
        systems = {}
        if location.startswith('http'):
            data = requests(location).text
        else:
            with open(location) as f:
                data = f.read()
        for line in data.splitlines():
            system = System(*line.split(';'))
            key = (system.dist, system.release, system.arch, system.variant)
            systems[key] = system
        return systems

    def get_updates(self, dist, release, arch, variant):
        old_system = self.systems[(dist, release, arch, variant)]
        url = '%s/%s/%s' % (SITE, INDEX_PATH, INDEX)
        new_systems = self.init_systems(url)
        new_system = new_systems[(dist, release, arch, variant)]
        if new_system.version > old_system.version:
            return new_system
        return None

    def get_lxc_data(self, system):
        image_path = os.path.join(self.workspace, system.path)
        os.makedirs(image_path)
        rootfs_path = os.path.join(self.workspace, system.path. ROOTFS)
        rootfs_url = '%s/%s/%s' % (SITE, system.path, ROOTFS)
        self.download(rootfs_url, rootfs_path)
        meta_path = os.path.join(self.workspace, system.path. META)
        meta_url = '%s/%s/%s' % (SITE, system.path, META)
        self.download(meta_url, meta_path)
        return rootfs_path, meta_path

    @staticmethod
    def download(url, path):
        request = requests.get(url, stream=True)
        if request.status_code == 200:
            with open(path, 'wb') as f:
                request.raw.decode_content = True
                shutil.copyfileobj(request.raw, f)

    def put_lxc_data(self, user_host, system, rootfs_path, meta_path):
        pass
        put_script = PUT_SCRIPT.format(
            user_host=user_host, rootfs_path=rootfs_path, meta_path=meta_path)
        subprocess.check_call([put_script], shell=True)
        install_script = INSTALL_SCRIPT.format(
            suer_host=user_host, lxc_cache=LXC_CACHE, rootfs=ROOTFS, meta=META)
        subprocess.check_call([install_script], shell=True)


def parse_args(argv=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser(
        "Update a remote host's download lxc template cache.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '--dist', default="ubuntu", help="The distribution to update.")
    parser.add_argument(
        '--variant', default="default", help="The variant to update.")
    parser.add_argument(
        'user-host', help='The user@host to update.')
    parser.add_argument(
        'release', help='The release to update.')
    parser.add_argument(
        'arch', help='The architcture of the remote host')
    parser.add_argument(
        'workspace', help='The path to the local dir to stage the update.')
    args = parser.parse_args(argv)
    return args


def main(argv):
    """Build and package juju for an non-debian OS."""
    args = parse_args(argv)
    try:
        lxc_cache = LxcCache(
            args.workspace, verbose=args.verbose, dry_run=args.dry_run)
        new_system = lxc_cache.get_updates(
            args.dist, args.release, args.arch, args.variant)
        if new_system:
            rootfs_path, meta_path = lxc_cache.get_lxc_data(new_system)
            lxc_cache.put_lxc_data(
                args.user_host, new_system, rootfs_path, meta_path)
    except Exception as e:
        print(e)
        print(getattr(e, 'output', ''))
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
