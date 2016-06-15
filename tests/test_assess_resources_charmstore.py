from contextlib import nested
import logging
from mock import patch
import StringIO
from tempfile import NamedTemporaryFile
from textwrap import dedent

import assess_resources_charmstore as arc
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
        with nested(
            patch.object(arc, 'configure_logging', autospec=True),
            patch.object(arc, 'assess_charmstore_resources', autospec=True),
        ) as (mock_cl, mock_cs_assess):
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
