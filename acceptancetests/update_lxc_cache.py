#!/usr/bin/python
"""Update the lxc 'download' template cache for hosts on closed networks."""

from __future__ import print_function

from argparse import ArgumentParser
from collections import namedtuple
import errno
import os
import sys
import traceback
import shutil
import subprocess
import urllib2


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
sudo mv ~/{rootfs} ~/{meta} {lxc_cache}
sudo chown -R root:root  {lxc_cache}
sudo tar -C {lxc_cache} -xf {lxc_cache}/meta.tar.xz
EOT
"""


class LxcCache:
    """Manage the LXC download template cache."""

    def __init__(self, workspace, verbose=False, dry_run=False):
        """Set the workspace for the local cache."""
        self.workspace = os.path.abspath(workspace)
        self.verbose = verbose
        self.dry_run = dry_run
        local_path = os.path.join(self.workspace, INDEX_PATH, INDEX)
        self.systems, ignore = self.init_systems(local_path)

    def init_systems(self, location):
        """Return a tuple of the dict of lxc Systems and the source data.

        A System has these attributes: 'dist', 'release', 'arch', 'variant',
        'version', and 'path'.  The dict keys are a tuple of
        (dist, release, arch, variant).
        """
        systems = {}
        if location.startswith('http'):
            request = urllib2.Request(location)
            response = urllib2.urlopen(request)
            data = response.read()
        else:
            try:
                with open(location) as f:
                    data = f.read()
            except IOError as e:
                if e.errno == errno.ENOENT:
                    if self.verbose:
                        print('Local cache is empty.')
                    return systems, None
        for line in data.splitlines():
            system = System(*line.split(';'))
            key = (system.dist, system.release, system.arch, system.variant)
            systems[key] = system
        return systems, data

    def get_updates(self, dist, release, arch, variant):
        """Return a tuple of the new system and the source data that match.

        The new system and source data will be None when there are
        no updates. The dist, release, arch, and variant args identify the
        system to return.
        """
        key = (dist, release, arch, variant)
        old_system = self.systems.get(key)
        url = '%s/%s/%s' % (SITE, INDEX_PATH, INDEX)
        new_systems, data = self.init_systems(url)
        new_system = new_systems[key]
        if not old_system or new_system.version > old_system.version:
            if self.verbose:
                print('Found new version for %s' % str(key))
                print(new_system.version)
            return new_system, data
        if self.verbose:
            print('Version is current for %s' % str(key))
            print(old_system.version)
        return None, None

    def get_lxc_data(self, system):
        """Download the system image and meta data.

        Return a tuple of the image and meta data paths.
        """
        image_path = os.path.join(self.workspace, system.path[1:])
        if not self.dry_run:
            if self.verbose:
                print('creating %s' % image_path)
            if not os.path.isdir(image_path):
                os.makedirs(image_path)
        rootfs_path = os.path.join(image_path, ROOTFS)
        rootfs_url = '%s%s%s' % (SITE, system.path, ROOTFS)
        self.download(rootfs_url, rootfs_path)
        meta_path = os.path.join(image_path, META)
        meta_url = '%s%s%s' % (SITE, system.path, META)
        self.download(meta_url, meta_path)
        return rootfs_path, meta_path

    def download(self, location, path):
        """Download a large binary from location to the specified path."""
        chunk = 16 * 1024
        if not self.dry_run:
            request = urllib2.Request(location)
            response = urllib2.urlopen(request)
            if response.getcode() == 200:
                with open(path, 'wb') as f:
                    shutil.copyfileobj(response, f, chunk)
                if self.verbose:
                    print('Downloaded %s' % location)

    def put_lxc_data(self, user_host, system, rootfs_path, meta_path):
        """Install the lxc image and meta data on the host.

        The user on the host must have password-less sudo.
        """
        lxc_cache = os.path.join(
            LXC_CACHE, system.dist, system.release, system.arch,
            system.variant)
        put_script = PUT_SCRIPT.format(
            user_host=user_host, rootfs_path=rootfs_path, meta_path=meta_path)
        if not self.dry_run:
            subprocess.check_call([put_script], shell=True)
            if self.verbose:
                print("Uploaded %s and %s" % (ROOTFS, META))
        install_script = INSTALL_SCRIPT.format(
            user_host=user_host, lxc_cache=lxc_cache, rootfs=ROOTFS, meta=META)
        if not self.dry_run:
            subprocess.check_call([install_script], shell=True)
            if self.verbose:
                print("Installed %s and %s" % (ROOTFS, META))

    def save_index(self, data):
        "Save the (current) index data for future calls to get_updates()."
        index_dir = os.path.join(self.workspace, INDEX_PATH)
        if not os.path.isdir(index_dir):
            os.makedirs(index_dir)
        index_path = os.path.join(self.workspace, INDEX_PATH, INDEX)
        with open(index_path, 'w') as f:
            f.write(data)
        if self.verbose:
            print('saved index: %s' % INDEX)


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
        'user_host', help='The user@host to update.')
    parser.add_argument(
        'release', help='The release to update.')
    parser.add_argument(
        'arch', help='The architecture of the remote host')
    parser.add_argument(
        'workspace', help='The path to the local dir to stage the update.')
    args = parser.parse_args(argv)
    return args


def main(argv):
    """Update the lxc download template cache for hosts on closed networks."""
    args = parse_args(argv)
    try:
        lxc_cache = LxcCache(
            args.workspace, verbose=args.verbose, dry_run=args.dry_run)
        new_system, data = lxc_cache.get_updates(
            args.dist, args.release, args.arch, args.variant)
        if new_system:
            rootfs_path, meta_path = lxc_cache.get_lxc_data(new_system)
            lxc_cache.put_lxc_data(
                args.user_host, new_system, rootfs_path, meta_path)
            lxc_cache.save_index(data)
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
