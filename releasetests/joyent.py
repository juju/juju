#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
from datetime import (
    datetime,
    timedelta,
)
from HTMLParser import HTMLParser
import json
import os
import pprint
from requests import (
    Request,
    Session,
)
import re
import subprocess
import sys
from textwrap import dedent
from time import sleep
import urllib2

from utils import until_timeout


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

STUCK_MACHINES_PATH = os.path.join(
    os.environ['HOME'], '.config/juju-release-tools/joyent-stuck-machines')
SUPPORT_HOST = 'https://help.joyent.com/'
EMAIL_FIELD_NAME = 'email'
SUBJECT_FIELD_NAME = 'ticket[subject]'
DESCRIPTION_FIELD_NAME = 'comment[value]'
SEVERITY_FIELD_NAME = 'ticket[fields][20980657]'
IP_ADDRESS_FIELD_NAME = 'ticket[fields][20915658]'
COMMENT_BODY_FIELD_NAME = 'a_comment_body'
# Form fields that are expected to exist in Joyent's support request form.
# The first five fields are hidden fields.
EXPECTED_FIELDS = set((
    'utf8', 'authenticity_token', 'ticket[ticket_form_id]', 'comment[uploads]',
    'ticket[via_followup_source_id]', EMAIL_FIELD_NAME, SUBJECT_FIELD_NAME,
    DESCRIPTION_FIELD_NAME, SEVERITY_FIELD_NAME, IP_ADDRESS_FIELD_NAME,
    COMMENT_BODY_FIELD_NAME))


class SupportRequestError(Exception):
    pass


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


class FormParser(HTMLParser):

    def __init__(self):
        HTMLParser.__init__(self)
        self.form_opened = False
        self.hidden_fields = {}
        self.visible_fields = []
        self.importance_field_opened = False
        self.importance_field_values = []

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == 'form':
            if attrs.get('name') == 'ticketform':
                self.form_opened = True
                self.post_url = attrs['action']
            return
        if not self.form_opened:
            return
        if tag in ('input', 'textarea'):
            if attrs.get('type') == 'hidden':
                self.hidden_fields[attrs['name']] = attrs.get('value')
            else:
                self.visible_fields.append(attrs['name'])
        elif tag == 'select':
            if attrs['name'] == SEVERITY_FIELD_NAME:
                self.visible_fields.append(attrs['name'])
                self.importance_field_opened = True
            else:
                print(
                    "Warning: Found unexpected <select> field in Joyent's "
                    "support form: {}".format(attrs['name']))
        elif tag == 'option' and self.importance_field_opened:
            self.importance_field_values.append(attrs['value'])

    def handle_endtag(self, tag):
        if tag == 'form':
            self.form_opened = False
        elif tag == 'select':
            self.importance_field_opened = False


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

    def send_stuck_machine_support_request(
            self, machine_id, machine_address, contact_mail_address):
        if self.dry_run:
            return
        session = Session()
        req = Request('GET', SUPPORT_HOST + 'anonymous_requests/new')
        resp = session.send(session.prepare_request(req))
        if resp.status_code != 200:
            raise SupportRequestError(
                'Got {} HTTP status reading the support form page'.format(
                    resp.status_code))
        parser = FormParser()
        parser.feed(resp.text)
        found_fields = set(parser.hidden_fields)
        found_fields.update(set(parser.visible_fields))
        if found_fields != EXPECTED_FIELDS:
            print(
                "Warning: Found field names differ from expected field names.")
            print("  not expected: {}".format(
                found_fields.difference(EXPECTED_FIELDS)))
            print("  missing: {}".format(
                EXPECTED_FIELDS.difference(found_fields)))
        if 'sev-2' not in parser.importance_field_values:
            print("Warning: Expected severity 'sev-2' not found in form data.")

        form_data = parser.hidden_fields
        form_data[EMAIL_FIELD_NAME] = contact_mail_address
        form_data[SUBJECT_FIELD_NAME] = 'Machine stuck in provisioning state'
        form_data[DESCRIPTION_FIELD_NAME] = dedent("""\
            Please delete the machine {} which is stuck in provisioning.

            Thank you
        """).format(machine_id)
        form_data[SEVERITY_FIELD_NAME] = 'sev-2'
        form_data[IP_ADDRESS_FIELD_NAME] = machine_address
        # A text field that is visibly hidden.
        form_data[COMMENT_BODY_FIELD_NAME] = ''

        req = Request(
            'POST', SUPPORT_HOST + parser.post_url, data=form_data)
        resp = session.send(session.prepare_request(req))
        if resp.status_code != 200:
            raise SupportRequestError(
                'Server response to support form POST: {}'.format(
                    resp.status_code))
        with open('joyent-form-response.html', 'w') as f:
            f.write(resp.text.encode('utf-8'))
        mo = re.search('<div id="error">(.*?)</div>', resp.text)
        if mo is not None:
            raise SupportRequestError(
                'Server sent error message: {}'.format(mo.group(1)))
        print("Deletion request for {} ({}) submitted.".format(
            machine_id, machine_address))
        if resp.text.find("You're almost done creating your request") < 0:
            print(
                "Warning: could not find the expected confirmation message "
                "in the support server's response.")

    def request_deletion(self, current_stuck, contact_mail_address):
        if os.path.exists(STUCK_MACHINES_PATH):
            with open(STUCK_MACHINES_PATH) as stuck_file:
                known_stuck_ids = set(json.load(stuck_file))
        else:
            known_stuck_ids = set()
        current_stuck_ids = set(machine['id'] for machine in current_stuck)
        new_stuck_ids = current_stuck_ids.difference(known_stuck_ids)
        if new_stuck_ids:
            new_stuck_machines = [
                machine for machine in current_stuck
                if machine['id'] in new_stuck_ids]
            for machine in new_stuck_machines:
                machine_id = machine['id']
                machine_address = machine.get('primaryIp')
                if not machine_address:
                    machine_address = 'n/a'
                if not self.dry_run:
                    self.send_stuck_machine_support_request(
                        machine_id, machine_address, contact_mail_address)
        stuck_machines_dir = os.path.split(STUCK_MACHINES_PATH)[0]
        if not self.dry_run:
            if not os.path.exists(stuck_machines_dir):
                os.makedirs(stuck_machines_dir)
            with open(STUCK_MACHINES_PATH, 'w') as stuck_file:
                json.dump(list(current_stuck_ids), stuck_file)

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

    def delete_old_machines(self, old_age, contact_mail_address):
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
            self.request_deletion(current_stuck, contact_mail_address)


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
    parser_delete_old_machine.add_argument(
        "contact_mail_address",
        help="Email address used in the Joyent support form")
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
        client.delete_old_machines(args.old_age, args.contact_mail_address)
    elif args.command == 'delete-old-objects':
        client.delete_old_objects(args.path, args.old_age)
    else:
        print("action not understood.")


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
