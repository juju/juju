#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
from datetime import (
    datetime,
    timedelta,
)
import json
import os
import pprint
import subprocess
import sys
from time import sleep
import urllib2

from utility import until_timeout


VERSION = '0.1.0'
USER_AGENT = "juju-cloud-tool/{} ({}) Python/{}".format(
    VERSION, sys.platform, sys.version.split(None, 1)[0])
ISO_8601_FORMAT = '%Y-%m-%dT%H:%M:%S.%fZ'


SSL_SIGN = """
echo -n "date:" {0} |
    openssl dgst -sha256 -sign {1} |
    openssl enc -e -a |
    tr -d '\n'
"""

OLD_MACHINE_AGE = 12


class DeleteRequest(urllib2.Request):

    def get_method(self):
        return "DELETE"


class HeadRequest(urllib2.Request):

    def get_method(self):
        return "HEAD"


class PostRequest(urllib2.Request):

    def get_method(self):
        return "POST"


class PutRequest(urllib2.Request):

    def get_method(self):
        return "PUT"


def parse_iso_date(string):
    return datetime.strptime(string, ISO_8601_FORMAT)


class Client:
    """A class that mirrors MantaClient without the modern Crypto.

    See https://github.com/joyent/python-manta
    """

    def __init__(self, sdc_url, account, key_id, key_path, manta_url,
                 user_agent=USER_AGENT, pause=3, dry_run=False, verbose=False):
        if sdc_url.endswith('/'):
            sdc_url = sdc_url[1:]
        self.sdc_url = sdc_url
        if manta_url.endswith('/'):
            manta_url = manta_url[1:]
        self.manta_url = manta_url
        self.account = account
        self.key_id = key_id
        self.key_path = key_path
        self.user_agent = user_agent
        self.pause = pause
        self.dry_run = dry_run
        self.verbose = verbose

    def make_request_headers(self, headers=None):
        """Return a dict of required headers.

        The Authorization header is always a signing of the "Date" header,
        where "date" must be lowercase.
        """
        timestamp = datetime.utcnow().strftime("%a, %d %b %Y %H:%M:%S GMT")
        script = SSL_SIGN.format(timestamp, self.key_path)
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

    def _request(self, path, method="GET", body=None, headers=None,
                 is_manta=False):
        headers = self.make_request_headers(headers)
        if path.startswith('/'):
            path = path[1:]
        if is_manta:
            base_url = self.manta_url
        else:
            base_url = self.sdc_url
        uri = "{}/{}/{}".format(base_url, self.account, path)
        if method == 'DELETE':
            request = DeleteRequest(uri, headers=headers)
        elif method == 'HEAD':
            request = HeadRequest(uri, headers=headers)
        elif method == 'POST':
            request = PostRequest(uri, data=body, headers=headers)
        elif method == 'PUT':
            request = PutRequest(uri, data=body, headers=headers)
        else:
            request = urllib2.Request(uri, headers=headers)
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

    def _list_objects(self, path, deep=False):
        headers, content = self._request(path, is_manta=True)
        objects = []
        for line in content.splitlines():
            obj = json.loads(line)
            obj['path'] = '%s/%s' % (path, obj['name'])
            objects.append(obj)
            if obj['type'] == 'directory' and deep:
                objects.extend(self._list_objects(obj['path'], deep=True))
        return objects

    def list_objects(self, path, deep=False):
        objects = self._list_objects(path, deep=deep)
        for obj in objects:
            print('{type:9} {mtime} {path}'.format(**obj))

    def delete_old_objects(self, path, old_age):
        now = datetime.utcnow()
        ago = timedelta(hours=old_age)
        objects = self._list_objects(path, deep=True)
        # The list is dir, the sub objects. Manta requires the sub objects
        # to be deleted first.
        objects.reverse()
        for obj in objects:
            if '.joyent' in obj['path']:
                # The .joyent dir cannot be deleted.
                print('ignoring %s' % obj['path'])
                continue
            mtime = parse_iso_date(obj['mtime'])
            age = now - mtime
            if age < ago:
                print('ignoring young %s' % obj['path'])
                continue
            if self.verbose:
                print('Deleting %s' % obj['path'])
            if not self.dry_run:
                headers, content = self._request(
                    obj['path'], method='DELETE', is_manta=True)

    def _list_machines(self, machine_id=None):
        """Return a list of machine dicts."""
        if machine_id:
            path = '/machines/{}'.format(machine_id)
        else:
            path = '/machines'
        headers, content = self._request(path)
        machines = json.loads(content)
        if self.verbose:
            print(machines)
        return machines

    def list_machines(self, machine_id=None):
        machines = self._list_machines(machine_id)
        pprint.pprint(machines, indent=2)

    def _list_machine_tags(self, machine_id):
        path = '/machines/{}/tags'.format(machine_id)
        headers, content = self._request(path)
        tags = json.loads(content)
        if self.verbose:
            print(tags)
        return tags

    def list_machine_tags(self, machine_id):
        tags = self._list_machine_tags(machine_id)
        pprint.pprint(tags, indent=2)

    def stop_machine(self, machine_id):
        path = '/machines/{}?action=stop'.format(machine_id)
        print("Stopping machine {}".format(machine_id))
        if not self.dry_run:
            headers, content = self._request(path, method='POST')

    def delete_machine(self, machine_id):
        path = '/machines/{}'.format(machine_id)
        print("Deleting machine {}".format(machine_id))
        if not self.dry_run:
            headers, content = self._request(path, method='DELETE')

    def attempt_deletion(self, current_stuck):
        all_success = True
        for machine_id in current_stuck:
            if self.verbose:
                print("Attempting to delete {} stuck in provisioning.".format(
                      machine_id))
            if not self.dry_run:
                try:
                    # Officially the we cannot delete non-stopped machines,
                    # but using the UI, we can delete machines stuck in
                    # provisioning or stopping, so we try.
                    self.delete_machine(machine_id)
                    if self.verbose:
                        print("Deleted {}".format(machine_id))
                except:
                    print('Delete stuck machine {} using the UI.'.format(
                          machine_id))
                    all_success = False
        return all_success

    def _delete_running_machine(self, machine_id):
        self.stop_machine(machine_id)
        for ignored in until_timeout(120):
            if self.verbose:
                print(".", end="")
                sys.stdout.flush()
            sleep(self.pause)
            stopping_machine = self._list_machines(machine_id)
            if stopping_machine['state'] == 'stopped':
                break
        if self.verbose:
            print("stopped")
        self.delete_machine(machine_id)

    def delete_old_machines(self, old_age):
        machines = self._list_machines()
        now = datetime.utcnow()
        current_stuck = []
        for machine in machines:
            created = parse_iso_date(machine['created'])
            age = now - created
            if age > timedelta(hours=old_age):
                machine_id = machine['id']
                tags = self._list_machine_tags(machine_id)
                if tags.get('permanent', 'false') == 'true':
                    continue
                if machine['state'] == 'provisioning':
                    current_stuck.append(machine)
                    continue
                if self.verbose:
                    print("Machine {} is {} old".format(machine_id, age))
                if not self.dry_run:
                    self._delete_running_machine(machine_id)
        if not self.dry_run and current_stuck:
            self.attempt_deletion(current_stuck)


def parse_args(argv=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser('Query and manage joyent.')
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", help='Increse verbosity.')
    parser.add_argument(
        "-u", "--url", dest="sdc_url",
        help="SDC URL. Environment: SDC_URL=URL",
        default=os.environ.get("SDC_URL"))
    parser.add_argument(
        "-m", "--manta-url", dest="manta_url",
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
        "-p", "--key-path", dest="key_path",
        help="Path to the SSH key",
        default=os.path.join(os.environ.get('JUJU_HOME', '~/.juju'), 'id_rsa'))
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    subparsers.add_parser('list-machines', help='List running machines')
    parser_delete_old_machine = subparsers.add_parser(
        'delete-old-machines',
        help='Delete machines older than %d hours' % OLD_MACHINE_AGE)
    parser_delete_old_machine.add_argument(
        '-o', '--old-age', default=OLD_MACHINE_AGE, type=int,
        help='Set old machine age to n hours.')
    parser_list_tags = subparsers.add_parser(
        'list-tags', help='List tags of running machines')
    parser_list_tags.add_argument('machine_id', help='The machine id.')
    parser_list_objects = subparsers.add_parser(
        'list-objects', help='List directories and files in manta')
    parser_list_objects.add_argument(
        '-r', '--recursive', action='store_true', default=False,
        help='Include content in subdirectories.')
    parser_list_objects.add_argument('path', help='The path')
    parser_delete_old_objects = subparsers.add_parser(
        'delete-old-objects',
        help='Delete objects older than %d hours' % OLD_MACHINE_AGE)
    parser_delete_old_objects.add_argument(
        '-o', '--old-age', default=OLD_MACHINE_AGE, type=int,
        help='Set old object age to n hours.')
    parser_delete_old_objects.add_argument('path', help='The path')

    args = parser.parse_args(argv)
    if not args.sdc_url:
        print('SDC_URL must be sourced into the environment.')
        sys.exit(1)
    return args


def main(argv):
    args = parse_args(argv)
    client = Client(
        args.sdc_url, args.account, args.key_id, args.key_path, args.manta_url,
        dry_run=args.dry_run, verbose=args.verbose)
    if args.command == 'list-machines':
        client.list_machines()
    elif args.command == 'list-tags':
        client.list_machine_tags(args.machine_id)
    elif args.command == 'list-objects':
        client.list_objects(args.path, deep=args.recursive)
    elif args.command == 'delete-old-machines':
        client.delete_old_machines(args.old_age)
    elif args.command == 'delete-old-objects':
        client.delete_old_objects(args.path, args.old_age)
    else:
        print("action not understood.")


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
