#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import base64
from datetime import datetime
import hashlib
import json
import mimetypes
import os
import subprocess
import sys
import urllib2

import manta


USER_AGENT = "juju-cloud-sync/{} ({}) Python/{}".format(
    manta.__version__, sys.platform, sys.version.split(None, 1)[0])


SSL_SIGN = """
echo -n "date:" {0} |
    openssl dgst -sha256 -sign {1} |
    openssl enc -e -a |
    tr -d '\n'
"""


class HeadRequest(urllib2.Request):

    def get_method(self):
        return "HEAD"


class PutRequest(urllib2.Request):

    def get_method(self):
        return "PUT"


class Client:
    """A class that mirrors MantaClient without the modern Crypto."""

    def __init__(self, manta_url, account, key_id,
                 user_agent=USER_AGENT, verbose=False):
        self.manta_url = manta_url
        self.account = account
        self.key_id = key_id
        self.user_agent = user_agent

    def make_request_headers(self, headers=None):
        """Return a dict of required headers.

        The Authorization header is always a signing of the "Date" header,
        where "date" must be lowercase.
        """
        timestamp = datetime.utcnow().strftime("%a, %d %b %Y %H:%M:%S GMT")
        key_path = os.path.join(os.environ['JUJU_HOME'], 'id_rsa')
        script = SSL_SIGN.format(timestamp, key_path)
        signature = subprocess.check_output(['bash', '-c', script])
        key = "/{}/keys/{}".format(self.account, self.key_id)
        auth = (
            'Signature keyId="{}",algorithm="rsa-sha256",'.format(key) +
            'signature="{}"'.format(signature))
        if headers is None:
            headers = {}
        headers['Date'] = timestamp
        headers['Authorization'] = auth
        headers["User-Agent"] = USER_AGENT
        return headers

    def _request(self, path, method="GET", body=None, headers=None):
        headers = self.make_request_headers(headers)
        container_url = "{}/{}".format(self.manta_url, path)
        if method == 'HEAD':
            request = HeadRequest(container_url, headers=headers)
        elif method == 'PUT':
            request = PutRequest(container_url, data=body, headers=headers)
        else:
            request = urllib2.Request(container_url, headers=headers)
        try:
            response = urllib2.urlopen(request)
        except Exception as err:
            print(request.header_items())
            print(err.read())
            raise
        content = response.read()
        headers = dict(response.headers.items())
        headers['status'] = str(response.getcode())
        headers['reason'] = response.msg
        return headers, content

    def ls(self, container_path):
        """Return a dict of a directory or file listing."""
        headers, content = self._request(container_path)
        files = {}
        for string in content.splitlines():
            data = json.loads(string)
            files[data['name']] = data
        return files

    def put_object(self, remote_path, path=None,
                   content_type="application/octet-stream",
                   durability_level=None):
        """Put an object at te remote path."""
        with open(path, mode='rb') as local_file:
            content = local_file.read()
        if not isinstance(content, bytes):
            raise ValueError("'content' must be bytes, not unicode")
        headers = {
            "Content-Type": content_type,
        }
        if durability_level:
            headers["x-durability-level"] = durability_level
        headers["Content-Length"] = str(len(content))
        headers["Content-MD5"] = get_md5content(path, content)
        response, content = self._request(
            remote_path, method="PUT", body=content, headers=headers)
        if response["status"] != "204":
            raise Exception(content)


def get_md5content(local_path, content=None):
    """Return the base64 encoded md5 digest for the local file."""
    if content is None:
        with open(local_path, mode='rb') as local_file:
            content = local_file.read()
    md5 = hashlib.md5(content)
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
    client = Client(
        args.manta_url, args.account, args.key_id, verbose=args.verbose)
    # /cpcjoyentsupport/public/juju-dist/tools/streams/v1/
    container_path = '/{0}/public/{1}/{2}'.format(
        args.account, args.container, args.path).replace('//', '/')
    if args.verbose:
        print(container_path)
    remote_files = get_files(container_path, client)
    upload_changes(args, remote_files, container_path, client)


if __name__ == '__main__':
    main()
