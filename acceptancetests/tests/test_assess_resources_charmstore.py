from contextlib import contextmanager
import logging
from mock import patch
import StringIO
from tempfile import NamedTemporaryFile
from textwrap import dedent

from fixtures import EnvironmentVariable

import assess_resources_charmstore as arc
from utility import JujuAssertionError
from tests import (
    TestCase,
    parse_error,
)


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = arc.parse_args(['/usr/bin/charm', 'credentials_file'])
        self.assertEqual('/usr/bin/charm', args.charm_bin)
        self.assertEqual('credentials_file', args.credentials_file)
        self.assertEqual(logging.INFO, args.verbose)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch('sys.stdout', fake_stdout):
                arc.parse_args(['--help'])
        self.assertEqual('', fake_stderr.getvalue())
        self.assertNotIn('TODO', fake_stdout.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = [
            '/usr/bin/charm',
            '/tmp/credentials_file',
            '--verbose',
            ]
        with patch.object(
                arc, 'configure_logging', autospec=True) as mock_cl:
            with patch.object(
                    arc,
                    'assess_charmstore_resources',
                    autospec=True) as mock_cs_assess:
                with EnvironmentVariable('JUJU_REPOSITORY', ''):
                    arc.main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cs_assess.assert_called_once_with(
            '/usr/bin/charm', '/tmp/credentials_file')


class TestCharmstoreDetails(TestCase):

    def test_raises_when_no_details_found(self):
        with NamedTemporaryFile() as tmp_file:
            self.assertRaises(
                ValueError, arc.get_charmstore_details, tmp_file.name)

    def test_creates_CharstoreDetails_object(self):
        cred_details = dedent("""\
        export STORE_CREDENTIALS="username@canonical.com:securepassword"
        export STORE_ADMIN="admin:messy-Travis"
        export STORE_URL="https://www.jujugui.org
        """)
        with NamedTemporaryFile() as tmp_file:
            tmp_file.write(cred_details)
            tmp_file.seek(0)

            results = arc.get_charmstore_details(tmp_file.name)
        self.assertEqual(results.email, 'username@canonical.com')
        self.assertEqual(results.username, 'username')
        self.assertEqual(results.password, 'securepassword')
        self.assertEqual(results.api_url, 'https://www.jujugui.org')

    def test_parse_credentials_file(self):
        cred_details = dedent("""\
        export STORE_CREDENTIALS="username.something@canonical.com:password"
        export STORE_ADMIN="admin:messy-Travis"
        export STORE_URL="https://www.jujugui.org
        """)
        with NamedTemporaryFile() as tmp_file:
            tmp_file.write(cred_details)
            tmp_file.seek(0)

            results = arc.parse_credentials_file(tmp_file.name)
        self.assertEqual(results['email'], 'username.something@canonical.com')
        self.assertEqual(results['username'], 'username-something')
        self.assertEqual(results['password'], 'password')
        self.assertEqual(results['api_url'], 'https://www.jujugui.org')

    def test_creates_CharstoreDetails_from_envvars(self):
        email = 'username@canonical.com'
        username = 'username'
        password = 'securepassword'
        api_url = 'https://www.jujugui.org'
        with set_charmstore_envvar(email, username, password, api_url):
            with NamedTemporaryFile() as tmp_file:
                results = arc.get_charmstore_details(tmp_file.name)
        self.assertEqual(results.email, email)
        self.assertEqual(results.username, username)
        self.assertEqual(results.password, password)
        self.assertEqual(results.api_url, api_url)

    def test_creates_CharstoreDetails_envvards_overwrite(self):
        cred_details = dedent("""\
        export STORE_CREDENTIALS="username.something@canonical.com:password"
        export STORE_ADMIN="admin:messy-Travis"
        export STORE_URL="https://www.jujugui.org
        """)
        email = 'username-env@canonical.com'
        username = 'username-env'
        password = 'password-env'
        api_url = 'https://www.jujugui.org-env'
        with set_charmstore_envvar(email, username, password, api_url):
            with NamedTemporaryFile() as tmp_file:
                tmp_file.write(cred_details)
                tmp_file.seek(0)
                results = arc.get_charmstore_details(tmp_file.name)
        self.assertEqual(results.email, email)
        self.assertEqual(results.username, username)
        self.assertEqual(results.password, password)
        self.assertEqual(results.api_url, api_url)


@contextmanager
def set_charmstore_envvar(email, username, password, api_url):
    with EnvironmentVariable('CS_EMAIL', email):
        with EnvironmentVariable('CS_USERNAME', username):
            with EnvironmentVariable('CS_PASSWORD', password):
                with EnvironmentVariable('CS_API_URL', api_url):
                    yield


class TestSplitLineDetails(TestCase):

    def test_doesnt_raise_when_no_split_token(self):
        self.assertEqual('', arc.split_line_details(''))

    def test_returns_string_free_of_quotes(self):
        base_string = '""'
        self.assertEqual('', arc.split_line_details(base_string))

    def test_returns_complete_token_from_config_string(self):
        base_string = 'something something = test_value'
        self.assertEqual(' test_value', arc.split_line_details(base_string))

    def test_ensure_removes_newlines(self):
        base_string = 'test_value\n'
        self.assertEqual('test_value', arc.split_line_details(base_string))


class TestRunId(TestCase):
    def test_replaces_characters_in_uuid(self):
        uuid = 'fbc4863a-3372-11e6-8aa3-0c8bfd6c5d2c'
        expected = 'fbc4863a337211e68aa30c8bfd6c5d2c'
        with patch.object(arc, 'uuid1', auto_spec=True, return_value=uuid):
            self.assertEquals(arc.get_run_id(), expected)


class TestRaiseIfContentsDiffer(TestCase):

    def test_raises_exception_on_mismatch(self):
        file_contents = 'abc'
        resource_contents = 'ab'
        self.assertRaises(
            JujuAssertionError,
            arc.raise_if_contents_differ,
            resource_contents,
            file_contents)

    def test_no_raise_on_contents_match(self):
        file_contents = resource_contents = 'ab'
        arc.raise_if_contents_differ(
            resource_contents=resource_contents,
            file_contents=file_contents)

    def test_exception_message(self):
        file_contents = 'abc'
        resource_contents = 'ab'
        expected_msg = dedent("""\
        Resource contents mismatch.
        Expected:
        {f}
        Got:
        {r}""".format(f=file_contents, r=resource_contents))
        with self.assertRaisesRegexp(JujuAssertionError, expected_msg):
            arc.raise_if_contents_differ(
                resource_contents=resource_contents,
                file_contents=file_contents)
