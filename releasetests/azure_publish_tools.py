#!/usr/bin/python
#
# The azure lib checks the environment for two vars that can be sourced
# or defined before the command.
# export AZURE_STORAGE_ACCOUNT=tcontepub
# export AZURE_STORAGE_ACCESS_KEY='secret key'

from __future__ import print_function

from argparse import ArgumentParser
import base64
from collections import namedtuple
import hashlib
import mimetypes
from operator import attrgetter
import os
import socket
import sys

from azure.storage.blob import (
    BlobBlock,
    BlockBlobService,
    ContentSettings,
    Include,
    )


mimetypes.init()


OK = 0
BAD_ARGS = 1
UNKNOWN_COMMAND = 2
NO_LOCAL_FILES = 4
UNKNOWN_PURPOSE = 5


LIST = 'list'
PUBLISH = 'publish'
DELETE = 'delete'
SYNC = 'sync'
RELEASED = 'released'
PROPOSED = 'proposed'
DEVEL = 'devel'
TESTING = 'testing'
WEEKLY = 'weekly'
PURPOSES = (RELEASED, PROPOSED, DEVEL, WEEKLY, TESTING)
JUJU_DIST = 'juju-tools'
CHUNK_SIZE = 4 * 1024 * 1024


SIGNED_EXTS = ('.sjson', '.gpg')


SyncFile = namedtuple(
    'SyncFile', ['path', 'size', 'md5content', 'mimetype', 'local_path'])


def get_prefix(purpose):
    """Return the top-level dir name for the purpose."""
    if purpose == RELEASED:
        return 'tools'
    else:
        return purpose


def get_published_files(purpose, blob_service):
    """Return the SyncFile info about the published files."""
    prefix = get_prefix(purpose)
    return list_sync_files(prefix, blob_service)


def list_sync_files(prefix, blob_service):
    """Return the SyncFile info about files under the specified prefix."""
    files = []
    for blob in blob_service.list_blobs(
            JUJU_DIST, prefix=prefix, include=Include.METADATA):
        sync_file = SyncFile(
            path=blob.name,
            md5content=blob.properties.content_settings.content_md5,
            size=blob.properties.content_length,
            mimetype=blob.properties.content_settings.content_type,
            local_path='')
        files.append(sync_file)
    return sorted(files, key=attrgetter('path'))


def get_local_files(purpose, local_dir):
    """Return SyncFile info about the files in the local tools tree."""
    if not os.path.isdir(local_dir):
        print('%s not found.' % local_dir)
        return None
    return [x for x in get_local_sync_files(get_prefix(purpose), local_dir) if
            'mirror' not in os.path.basename(x.local_path)]


def get_local_sync_files(prefix, local_dir):
    replacements = (local_dir, prefix)
    found = []
    for path, subdirs, files in os.walk(local_dir):
        for name in files:
            local_path = os.path.join(path, name)
            publish_path = local_path.replace(*replacements)
            if os.path.islink(local_path):
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


def publish_local_file(blob_service, sync_file):
    """Published the local file to the remote location.

    The file is broken down into blocks that can be uploaded within
    the azure restrictions. The blocks are then assembled into a blob
    with the md5 content (base64 encoded digest).
    """
    block_ids = []
    index = 0
    with open(sync_file.local_path, 'rb') as local_file:
        while True:
            data = local_file.read(CHUNK_SIZE)
            if data:
                block_id = base64.b64encode(str(index))
                for i in range(0, 3):
                    try:
                        blob_service.put_block(
                            JUJU_DIST, sync_file.path, data, block_id)
                        block_ids.append(BlobBlock(id=block_id))
                        index += 1
                        break
                    except socket.error as e:
                        if e.errno not in (socket.errno.ECONNREFUSED,
                                           socket.errno.ENETUNREACH,
                                           socket.errno.ETIMEDOUT):
                            raise
            else:
                break
    for i in range(0, 3):
        try:
            content_settings = ContentSettings(
                content_type=sync_file.mimetype,
                content_md5=sync_file.md5content)
            blob_service.put_block_list(
                JUJU_DIST, sync_file.path, block_ids,
                content_settings=content_settings)
            break
        except socket.error as e:
            if e.errno not in (socket.errno.ECONNREFUSED,
                               socket.errno.ENETUNREACH,
                               socket.errno.ETIMEDOUT):
                raise


def list_published_files(blob_service, purpose):
    """List the files specified by the purpose."""
    for sync_file in get_published_files(purpose, blob_service):
        print(
            '%s %s %s' % (
                sync_file.path, sync_file.size, sync_file.md5content))
    return OK


def normalized_dir(local_dir):
    if local_dir.endswith('/'):
        local_dir = local_dir[:-1]
    return local_dir


def publish_files(blob_service, purpose, local_dir, args):
    """Publish the streams to the location for the intended purpose."""
    local_dir = normalized_dir(local_dir)
    print("Looking for published files in %s" % purpose)
    published_files = get_published_files(purpose, blob_service)
    print("Looking for local files in %s" % local_dir)
    local_files = get_local_files(purpose, local_dir)
    if local_files is None:
        if args.verbose:
            print("No files were found at {}".format(local_dir))
        return NO_LOCAL_FILES
    return publish_changed(blob_service, local_files, published_files, args)


def sync_files(blob_service, prefix, local_dir, args):
    local_dir = normalized_dir(local_dir)
    print("Looking for published files in %s" % prefix)
    sync_files = list_sync_files(prefix, blob_service)
    print("Looking for local files in %s" % local_dir)
    local_files = get_local_sync_files(prefix, local_dir)
    if local_files == []:
        if args.verbose:
            print("No files were found at {}".format(local_dir))
        return NO_LOCAL_FILES
    return publish_changed(blob_service, local_files, sync_files, args)


def publish_changed(blob_service, local_files, remote_files, args):
    if args.verbose:
        for lf in local_files:
            print(lf.path)
    published_dict = dict(
        (sync_file.path, sync_file) for sync_file in remote_files)
    for sync_file in local_files:
        if sync_file.path not in published_dict:
            print('%s is new.' % sync_file.path)
        elif published_dict[sync_file.path].md5content != sync_file.md5content:
            print('%s is different.' % sync_file.path)
            if args.verbose:
                print(
                    '  published:%s != local:%s.' % (
                        published_dict[sync_file.path].md5content,
                        sync_file.md5content))
        else:
            if args.verbose:
                print("Nothing to do: %s == %s" % (
                    sync_file.path, published_dict[sync_file.path].md5content))
            continue
        if not args.dry_run:
            publish_local_file(blob_service, sync_file)
    return OK


def delete_files(blob_service, purpose, files, args):
    prefix = get_prefix(purpose)
    for path in files:
        if prefix is not None:
            path = '%s/%s' % (prefix, path)
        print("Deleting %s" % path)
        if not args.dry_run:
            blob_service.delete_blob(JUJU_DIST, path)
    return OK


def main():
    """Execute the commands from the command line."""
    parser = get_option_parser()
    args = parser.parse_args()
    blob_service = BlockBlobService(
        account_name=args.account_name, account_key=args.account_key)
    if args.command == SYNC:
        return sync_files(blob_service, args.prefix, args.local_dir, args)
    if args.purpose not in PURPOSES:
        print('Unknown purpose: {}'.format(args.purpose))
        return UNKNOWN_PURPOSE
    if args.command == LIST:
        return list_published_files(blob_service, args.purpose)
    elif args.command == PUBLISH:
        stream_path = os.path.join(args.path, get_prefix(args.purpose))
        return publish_files(blob_service, args.purpose, stream_path, args)
    elif args.command == DELETE:
        if args.path is None:
            parser.print_usage()
            return BAD_ARGS
        return delete_files(blob_service, args.purpose, args.path, args)


def add_mutation_args(parser, dr_help):
    parser.add_argument("-d", "--dry-run", action="store_true",
                        default=False, help=dr_help)
    parser.add_argument('-v', '--verbose', action="store_true",
                        default=False, help='Increase verbosity.')


def get_option_parser():
    """Return the option parser for this program."""
    parser = ArgumentParser(description="Manage objects in Azure blob storage")
    parser.add_argument(
        '--account-name',
        default=os.environ.get('AZURE_STORAGE_ACCOUNT', None),
        help="The azure storage account, or env AZURE_STORAGE_ACCOUNT.")
    parser.add_argument(
        '--account-key',
        default=os.environ.get('AZURE_STORAGE_ACCESS_KEY', None),
        help="The azure storage account, or env AZURE_STORAGE_ACCESS_KEY.")
    subparsers = parser.add_subparsers(help='sub-command help', dest='command')
    for command in (LIST, PUBLISH, DELETE):
        subparser = subparsers.add_parser(command, help='Command to run')
        subparser.add_argument(
            'purpose', help="<{}>".format(' | '.join(PURPOSES)))
        if command == PUBLISH:
            subparser.add_argument('path', help='The path to publish')
            add_mutation_args(subparser, 'Do not publish')
        elif command == DELETE:
            subparser.add_argument('path', nargs="+",
                                   help='The files to delete')
            add_mutation_args(subparser, 'Do not delete')
    sync_parser = subparsers.add_parser('sync',
                                        help='Sync local files to remote.')
    sync_parser.add_argument('local_dir', metavar='LOCAL-DIR',
                             help='The local directory to sync')
    sync_parser.add_argument('prefix', metavar='PREFIX',
                             help='The remote prefix to sync to')
    add_mutation_args(sync_parser, 'Do not sync')
    return parser


if __name__ == '__main__':
    sys.exit(main())
