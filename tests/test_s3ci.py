from argparse import Namespace
from ConfigParser import NoOptionError
from tempfile import NamedTemporaryFile
from textwrap import dedent
from unittest import TestCase
from StringIO import StringIO

from mock import patch

from s3ci import (
    get_s3_credentials,
    main,
    parse_args,
    )
from tests import (
    parse_error,
    stdout_guard,
    use_context,
    )


class TestParseArgs(TestCase):

    def test_get_juju_bin_defaults(self):
        args = parse_args(['get-juju-bin', 'myconfig', '3275'])
        self.assertEqual(Namespace(
            command='get-juju-bin', config='myconfig', revision_build=3275,
            workspace='.'),
            args)

    def test_get_juju_bin_workspace(self):
        args = parse_args(['get-juju-bin', 'myconfig', '3275', 'myworkspace'])
        self.assertEqual('myworkspace', args.workspace)

    def test_get_juju_bin_too_few(self):
        with parse_error(self) as stderr:
            parse_args(['get-juju-bin', 'myconfig'])
        self.assertRegexpMatches(stderr.getvalue(), 'too few arguments$')


class TestGetS3Credentials(TestCase):

    def test_get_s3_credentials(self):
        with NamedTemporaryFile() as temp_file:
            temp_file.write(dedent("""\
                [default]
                access_key = fake_username
                secret_key = fake_pass
                """))
            temp_file.flush()
            access_key, secret_key = get_s3_credentials(temp_file.name)
        self.assertEqual(access_key, "fake_username")
        self.assertEqual(secret_key, "fake_pass")

    def test_no_access_key(self):
        with NamedTemporaryFile() as temp_file:
            temp_file.write(dedent("""\
                [default]
                secret_key = fake_pass
                """))
            temp_file.flush()
            with self.assertRaisesRegexp(
                    NoOptionError,
                    "No option 'access_key' in section: 'default'"):
                get_s3_credentials(temp_file.name)

    def test_get_s3_access_no_secret_key(self):
        with NamedTemporaryFile() as temp_file:
            temp_file.write(dedent("""\
                [default]
                access_key = fake_username
                """))
            temp_file.flush()
            with self.assertRaisesRegexp(
                    NoOptionError,
                    "No option 'secret_key' in section: 'default'"):
                get_s3_credentials(temp_file.name)


class TestMain(TestCase):

    def setUp(self):
        use_context(self, stdout_guard())

    def test_main_args(self):
        stdout = StringIO()
        with NamedTemporaryFile() as temp_file:
            temp_file.write(dedent("""\
                [default]
                access_key = fake_username
                secret_key = fake_pass
                """))
            temp_file.flush()
            with patch('sys.argv', [
                    'foo', 'get-juju-bin', temp_file.name, '28',
                    'bar-workspace']):
                with patch('s3ci.S3Connection', autospec=True) as s3c_mock:
                    with patch('s3ci.get_juju_bin', autospec=True,
                               return_value='gjb') as gbj_mock:
                        with patch('sys.stdout', stdout):
                            main()
        s3c_mock.assert_called_once_with('fake_username', 'fake_pass')
        gb_mock = s3c_mock.return_value.get_bucket
        gb_mock.assert_called_once_with('juju-qa-data')
        gbj_mock.assert_called_once_with(gb_mock.return_value, 28,
                                         'bar-workspace')
        self.assertEqual('gjb\n', stdout.getvalue())
