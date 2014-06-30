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


JOYENT_PROCS = "ps ax -eo pid,etime,command | grep joyent | grep juju"


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


class Client:
    """A class that mirrors MantaClient without the modern Crypto.

    See https://github.com/joyent/python-manta
    """

    def __init__(self, sdc_url, account, key_id,
                 user_agent=USER_AGENT, verbose=False):
        self.sdc_url = sdc_url
        self.account = account
        self.key_id = key_id
        self.user_agent = user_agent
        self.verbose = verbose

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
        container_url = "{}/{}/{}".format(self.sdc_url, self.account, path)
        if method == 'DELETE':
            request = DeleteRequest(container_url, headers=headers)
        if method == 'HEAD':
            request = HeadRequest(container_url, headers=headers)
        elif method == 'POST':
            request = PostRequest(container_url, data=body, headers=headers)
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
        headers, content = self._request(path, method='POST')

    def delete_machine(self, machine_id):
        path = '/machines/{}'.format(machine_id)
        print("Deleting machine {}".format(machine_id))
        headers, content = self._request(path, method='DELETE')

    def delete_old_machines(self):
        procs = subprocess.check_output(['bash', '-c', JOYENT_PROCS])
        for proc in procs.splitlines():
            command = proc.split()
            pid = command.pop(0)
            alive = command.pop(0)
            if len(alive) > 5 and int(alive.split(':')[0]) > 0:
                # the pid has an hours column and the value is greater than 1.
                print("Pid {} is {} old. Ending {}".format(pid, alive, command))
                subprocess.check_output(['kill', '-9', pid])
        machines = self._list_machines()
        now = datetime.utcnow()
        for machine in machines:
            created = datetime.strptime(machine['created'], ISO_8601_FORMAT)
            age = now - created
            print(age)
            if age > timedelta(hours=1):
                machine_id = machine['id']
                print("Machine {} is {} old".format(machine_id, age))
                self.stop_machine(machine_id)
                while True:
                    print(".", end="")
                    sys.stdout.flush()
                    sleep(3)
                    stopping_machine = self._list_machines(machine_id)
                    if stopping_machine['state'] == 'stopped':
                        break
                print("stopped")
                self.delete_machine(machine_id)




def main():
    parser = ArgumentParser('Query and manage joyent.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", help='Increse verbosity.')
    parser.add_argument(
        "-u", "--url", dest="sdc_url",
        help="SDC URL. Environment: SDC_URL=URL",
        default=os.environ.get("SDC_URL"))
    parser.add_argument(
        "-a", "--account",
        help="Manta account. Environment: MANTA_USER=ACCOUNT",
        default=os.environ.get("MANTA_USER"))
    parser.add_argument(
        "-k", "--key-id", dest="key_id",
        help="SSH key fingerprint.  Environment: MANTA_KEY_ID=FINGERPRINT",
        default=os.environ.get("MANTA_KEY_ID"))
    parser.add_argument('action', help='The action to perform.')
    parser.add_argument('machine_id', help='The machine id.', nargs="?", default=None)
    args = parser.parse_args()
    if not args.sdc_url:
        print('SDC_URL must be sourced into the environment.')
        sys.exit(1)
    client = Client(
        args.sdc_url, args.account, args.key_id, verbose=args.verbose)
    if args.action == 'list-machines':
        client.list_machines()
    elif args.action == 'list-tags':
        client.list_machine_tags(args.machine_id)
    elif args.action == 'delete-old-machines':
        client.delete_old_machines()
    else:
        print("action not understood.")


if __name__ == '__main__':
    main()
