#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import base64
import hashlib
import mimetypes
import os
import sys

import manta


USER_AGENT = "juju-cloud-sync/{} ({}) Python/{}".format(
    manta.__version__, sys.platform, sys.version.split(None, 1)[0])


def get_md5content(remote_path):
    """Return the base64 encoded md5 digest for the local file."""
    md5 = hashlib.md5()
    with open(remote_path, mode='rb') as local_file:
        md5.update(local_file.read())
    base64_md5 = base64.encodestring(md5.digest()).strip()
    return base64_md5


def get_files(container_path, client):
    remote_files = client.ls(container_path)
    for file_name in remote_files:
        file_path = "{0}/{1}".format(container_path, file_name)
        for i in range(3):
            response, content = client._request(file_path, "HEAD")
            if response["status"] == "200":
                break
        else:
            raise Exception(response)
        remote_files[file_name].update(response)
    return remote_files


def upload_changes(args, remote_files, container_path, client):
    count = 0
    if args.verbose:
        print("Thes container has: {}".format(remote_files.keys()))
    for file_name in args.files:
        remote_path = "{0}/{1}".format(
            container_path, file_name).replace('//', '/')
        remote_file = remote_files.get(file_name)
        if remote_file is None:
            if args.verbose:
                print("File is new: {0}".format(remote_path))
        else:
            remote_hash = str(remote_file['content-md5'])
            local_hash = get_md5content(file_name)
            if remote_hash == local_hash:
                if args.verbose:
                    print("File is same: {0}".format(remote_path))
                continue
            else:
                if args.verbose:
                    print("File is different: {0}".format(remote_path))
                    print("  {0} != {1}".format(local_hash, remote_hash))
        count += 1
        print("Uploading {0}".format(remote_path))
        content_type = (
            mimetypes.guess_type(file_name)[0] or "application/octet-stream")
        client.put_object(
            remote_path, path=file_name, content_type=content_type)
    print('Uploaded {0} files'.format(count))


def main():
    parser = ArgumentParser('Sync changed and new files.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", help='Increse verbosity.')
    parser.add_argument(
        "-u", "--url", dest="manta_url",
        help="Manta URL. Environment: MANTA_URL=URL",
        default=os.environ.get("MANTA_URL"))
    parser.add_argument(
        "-a", "--account",
        help="Manta account. Environment: MANTA_USER=ACCOUNT",
        default=os.environ.get("MANTA_USER"))
    parser.add_argument(
        "-k", "--key-id", dest="key_id",
        help="SSH key fingerprint.  Environment: MANTA_KEY_ID=FINGERPRINT",
        default=os.environ.get("MANTA_KEY_ID"))
    parser.add_argument(
        '--container', default='juju-dist', help='The container name.')
    parser.add_argument('path', help='The destination path in the container.')
    parser.add_argument(
        'files', nargs='*', help='The files to send to the container.')
    args = parser.parse_args()
    if not args.manta_url:
        print('MANTA_URL must be sourced into the environment.')
        sys.exit(1)
    client = manta.MantaClient(
        args.manta_url, args.account, signer=manta.CLISigner(args.key_id),
        user_agent=USER_AGENT, verbose=args.verbose)
    # /cpcjoyentsupport/public/juju/tools/streams/v1/
    container_path = '/{0}/public/{1}/{2}'.format(
        args.account, args.container, args.path).replace('//', '/')
    if args.verbose:
        print(container_path)
    remote_files = get_files(container_path, client)
    upload_changes(args, remote_files, container_path, client)


if __name__ == '__main__':
    main()
