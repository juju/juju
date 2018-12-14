# Copyright 2014-2015 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import tarfile
import zipfile
from charmhelpers.core import (
    host,
    hookenv,
)


class ArchiveError(Exception):
    pass


def get_archive_handler(archive_name):
    if os.path.isfile(archive_name):
        if tarfile.is_tarfile(archive_name):
            return extract_tarfile
        elif zipfile.is_zipfile(archive_name):
            return extract_zipfile
    else:
        # look at the file name
        for ext in ('.tar', '.tar.gz', '.tgz', 'tar.bz2', '.tbz2', '.tbz'):
            if archive_name.endswith(ext):
                return extract_tarfile
        for ext in ('.zip', '.jar'):
            if archive_name.endswith(ext):
                return extract_zipfile


def archive_dest_default(archive_name):
    archive_file = os.path.basename(archive_name)
    return os.path.join(hookenv.charm_dir(), "archives", archive_file)


def extract(archive_name, destpath=None):
    handler = get_archive_handler(archive_name)
    if handler:
        if not destpath:
            destpath = archive_dest_default(archive_name)
        if not os.path.isdir(destpath):
            host.mkdir(destpath)
        handler(archive_name, destpath)
        return destpath
    else:
        raise ArchiveError("No handler for archive")


def extract_tarfile(archive_name, destpath):
    "Unpack a tar archive, optionally compressed"
    archive = tarfile.open(archive_name)
    archive.extractall(destpath)


def extract_zipfile(archive_name, destpath):
    "Unpack a zip file"
    archive = zipfile.ZipFile(archive_name)
    archive.extractall(destpath)
