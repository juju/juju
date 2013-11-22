#!/usr/bin/python
#
# The azure lib checks the environment for two vars that can be sourced
# or defined before the command.
# export AZURE_STORAGE_ACCOUNT=tcontepub
# export AZURE_STORAGE_ACCESS_KEY='secret key'

from __future__ import print_function

import base64
from collections import namedtuple
import hashlib
import mimetypes
from operator import attrgetter
from optparse import OptionParser
import os
import sys

from azure.storage import BlobService


mimetypes.init()


OK = 0
BAD_ARGS = 1
UNKNOWN_COMMAND = 2
NO_PUBLISHED_FILES = 3
NO_LOCAL_FILES = 4


LIST = 'list'
PUBLISH = 'publish'
RELEASE = 'release'
TESTING = 'testing'
JUJU_DIST = 'juju-tools'
CHUNK_SIZE = 4 * 1024 * 1024


SyncFile = namedtuple(
    'SyncFile', ['path', 'size', 'md5content', 'mimetype', 'local_path'])


def get_published_files(purpose, blob_service):
    """Return the SyncFile info about the published files."""
    if purpose == TESTING:
        prefix = TESTING
    else:
        prefix = None
    files = []
    for blob in blob_service.list_blobs(
            JUJU_DIST, prefix=prefix, include='metadata'):
        if purpose == RELEASE and blob.name.startswith(TESTING):
            continue
        sync_file = SyncFile(
            path=blob.name, md5content=blob.properties.content_md5,
            size=blob.properties.content_length,
            mimetype=blob.properties.content_type, local_path='')
        files.append(sync_file)
    return sorted(files, key=attrgetter('path'))


def get_local_files(purpose, local_dir):
    """Return SyncFile info about the files in the local tools tree."""
    if not os.path.isdir(local_dir):
        print('%s not found.' % local_dir)
        return None
    if purpose == TESTING:
        replacements = (local_dir, TESTING)
    else:
        replacements = (local_dir, '')
    found = []
    for path, subdirs, files in os.walk(local_dir):
        for name in files:
            local_path = os.path.join(path, name)
            publish_path = local_path.replace(*replacements)
            if 'mirror' in name or os.path.islink(local_path):
                # The mirror files only belong on streams.canonical.com.
                continue
            size = os.path.getsize(local_path)
            md5content = get_md5content(local_path)
            mimetype, encoding = mimetypes.guess_type(name)
            sync_file = SyncFile(
                path=publish_path, size=size, md5content=md5content,
                mimetype=mimetype, local_path=local_path)
            found.append(sync_file)
    return sorted(found, key=attrgetter('path'))


def get_md5content(local_path):
    """Return the base64 encoded md5 digest for the local file."""
    md5 = hashlib.md5()
    with open(local_path, mode='rb') as local_file:
        md5.update(local_file.read())
    base64_md5 = base64.encodestring(md5.digest()).strip()
    return base64_md5


def publish_local_file(purpose, blob_service, sync_file):
    """Published the local file to the release or testing location.

    The file is broken down into blocks that can be uploaded within
    the azure restrictions. The blocks are then assembled into a blob
    with the md5 content (base64 encoded digest).
    """
    blob_service.put_blob(JUJU_DIST, sync_file.path, '', 'BlockBlob')
    block_ids = []
    index = 0
    with open(sync_file.local_path, 'rb') as local_file:
        while True:
            data = local_file.read(CHUNK_SIZE)
            if data:
                block_id = base64.b64encode(str(index))
                blob_service.put_block(
                    JUJU_DIST, sync_file.path, data, block_id)
                block_ids.append(block_id)
                index += 1
            else:
                break
    blob_service.put_block_list(
        JUJU_DIST, sync_file.path, block_ids,
        x_ms_blob_content_type=sync_file.mimetype,
        x_ms_blob_content_md5=sync_file.md5content)


def list_published_files(purpose):
    """List the testing or release files."""
    blob_service = BlobService()
    published_files = get_published_files(purpose, blob_service)
    if published_files is None:
        return NO_PUBLISHED_FILES
    for sync_file in published_files:
        print(
            '%s %s %s' % (
                sync_file.path, sync_file.size, sync_file.md5content))
    return OK


def publish_files(purpose, local_dir, options):
    """Publish the tools and metadata to the release or testing location."""
    blob_service = BlobService()
    if local_dir.endswith('/'):
        local_dir = local_dir[:-1]
    print("Looking for published files in %s" % purpose)
    published_files = get_published_files(purpose, blob_service)
    if published_files is None:
        return NO_PUBLISHED_FILES
    print("Looking for local files in %s" % local_dir)
    local_files = get_local_files(purpose, local_dir)
    if local_files is None:
        return NO_LOCAL_FILES
    published_dict = dict(
        (sync_file.path, sync_file) for sync_file in published_files)
    for sync_file in local_files:
        if sync_file.path not in published_dict:
            print('%s is new.' % sync_file.path)
        elif published_dict[sync_file.path].md5content != sync_file.md5content:
            print('%s is different.' % sync_file.path)
            if options.verbose:
                print(
                    '  published:%s != local:%s.' % (
                        published_dict[sync_file.path].md5content,
                        sync_file.md5content))
        else:
            continue
        if not options.dry_run:
            publish_local_file(purpose, blob_service, sync_file)
    return OK


def main():
    """Execute the commands from the command line."""
    parser = get_option_parser()
    (options, args) = parser.parse_args(args=sys.argv[1:])
    if len(args) <= 1:
        parser.print_usage()
        return BAD_ARGS
    command = args[0]
    purpose = args[1]
    if command == LIST:
        return list_published_files(purpose)
    elif command == PUBLISH:
        if len(args) != 3:
            parser.print_usage()
            return 1
        return publish_files(purpose, args[2], options)
    else:
        # The command is not known.
        return UNKNOWN_COMMAND


def get_option_parser():
    """Return the option parser for this program."""
    usage = "usage: %prog <list | publish> <testing | release> [local-tools]"
    parser = OptionParser(usage=usage)
    parser.add_option(
        "-d", "--dry-run", action="store_true", dest="dry_run")
    parser.add_option(
        "-v", "--verbose", action="store_true", dest="verbose")
    parser.set_defaults(
        dry_run=False,
        verbose=False
    )
    return parser


if __name__ == '__main__':
    sys.exit(main())
