from argparse import Namespace
from ConfigParser import NoOptionError
from contextlib import contextmanager
import errno
import os
from StringIO import StringIO
import sys
from tempfile import NamedTemporaryFile
from textwrap import dedent
from unittest import (
    skipIf,
    TestCase
    )

from boto.s3.bucket import Bucket
from boto.s3.key import Key as S3Key
from mock import (
    create_autospec,
    patch,
    )

from jujuci import (
    JobNamer,
    PackageNamer,
    )
from jujuconfig import get_juju_home
from s3ci import (
    fetch_files,
    fetch_juju_binary,
    find_file_keys,
    find_package_key,
    get_job_path,
    get_s3_credentials,
    JUJU_QA_DATA,
    main,
    PackageNotFound,
    parse_args,
    )
from tests import (
    parse_error,
    stdout_guard,
    TestCase as StrictTestCase,
    use_context,
    )
from utility import temp_dir


class TestParseArgs(TestCase):

    def test_get_juju_bin_defaults(self):
        default_config = os.path.join(get_juju_home(), 'juju-qa.s3cfg')
        args = parse_args(['get-juju-bin', '3275'])
        self.assertEqual(Namespace(
            command='get-juju-bin', config=default_config, revision_build=3275,
            workspace='.', verbose=0),
            args)

    def test_get_juju_bin_workspace(self):
        args = parse_args(['get-juju-bin', '3275', 'myworkspace'])
        self.assertEqual('myworkspace', args.workspace)

    def test_get_juju_bin_too_few(self):
        with parse_error(self) as stderr:
            parse_args(['get-juju-bin'])
        self.assertRegexpMatches(stderr.getvalue(), 'too few arguments$')

    def test_get_juju_bin_verbosity(self):
        args = parse_args(['get-juju-bin', '3275', '-v'])
        self.assertEqual(1, args.verbose)
        args = parse_args(['get-juju-bin', '3275', '-vv'])
        self.assertEqual(2, args.verbose)

    def test_get_defaults(self):
        default_config = os.path.join(get_juju_home(), 'juju-qa.s3cfg')
        args = parse_args(['get', '3275', 'job-foo', 'files-bar'])
        self.assertEqual(Namespace(
            command='get', config=default_config, revision_build=3275,
            job='job-foo', file_pattern='files-bar', workspace='.', verbose=0),
            args)

    def test_get_workspace(self):
        args = parse_args(['get', '3275', 'job-foo', 'files-bar',
                           'myworkspace'])
        self.assertEqual('myworkspace', args.workspace)

    def test_get_too_few(self):
        with parse_error(self) as stderr:
            parse_args(['get', '3275', 'job-foo'])
        self.assertRegexpMatches(stderr.getvalue(), 'too few arguments$')

    def test_get_verbosity(self):
        args = parse_args(['get', '3275', 'job-foo', 'files-bar', '-v'])
        self.assertEqual(1, args.verbose)
        args = parse_args(['get', '3275', 'job-foo', 'files-bar', '-vv'])
        self.assertEqual(2, args.verbose)


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

    def test_get_s3_credentials_missing(self):
        with self.assertRaises(IOError) as exc:
            get_s3_credentials('/asdf')
        self.assertEqual(exc.exception.errno, errno.ENOENT)


def mock_key(revision_build, job, build, file_path):
    key = create_autospec(S3Key, instance=True)
    key.name = '{}/build-{}/{}'.format(
        get_job_path(revision_build, job), build, file_path)
    return key


def mock_package_key(revision_build, build=27, distro_release=None):
    namer = PackageNamer.factory()
    if distro_release is not None:
        namer.distro_release = distro_release
    package = namer.get_release_package('109.6')
    job = JobNamer.factory().get_build_binary_job()
    return mock_key(revision_build, job, build, package)


def mock_bucket(keys):
    bucket = create_autospec(Bucket, instance=True)

    def list_bucket(prefix):
        return [key for key in keys if key.name.startswith(prefix)]

    bucket.list.side_effect = list_bucket
    return bucket


def get_key_filename(key):
    return key.name.split('/')[-1]


@skipIf(sys.platform in ('win32', 'darwin'),
        'Not supported on Windows and OS X')
class TestFindPackageKey(StrictTestCase):

    def setUp(self):
        use_context(self, patch('utility.get_deb_arch', return_value='amd65',
                                autospec=True))

    def test_find_package_key(self):
        key = mock_package_key(390)
        bucket = mock_bucket([key])
        namer = JobNamer.factory()
        job = namer.get_build_binary_job()
        found_key, filename = find_package_key(bucket, 390)
        bucket.list.assert_called_once_with(get_job_path(390, job))
        self.assertIs(key, found_key)
        self.assertEqual(filename, get_key_filename(key))

    def test_selects_latest(self):
        new_key = mock_package_key(390, build=27)
        old_key = mock_package_key(390, build=9)
        bucket = FakeBucket([old_key, new_key, old_key])
        found_key = find_package_key(bucket, 390)[0]
        self.assertIs(new_key, found_key)

    def test_wrong_version(self):
        key = mock_package_key(390, distro_release='01.01')
        bucket = FakeBucket([key])
        with self.assertRaises(PackageNotFound):
            find_package_key(bucket, 390)

    def test_wrong_file(self):
        key = mock_package_key(390)
        key.name = key.name.replace('juju-core', 'juju-dore')
        bucket = FakeBucket([key])
        with self.assertRaises(PackageNotFound):
            find_package_key(bucket, 390)


@skipIf(sys.platform in ('win32', 'darwin'),
        'Not supported on Windows and OS X')
class TestFetchJujuBinary(StrictTestCase):

    def setUp(self):
        use_context(self, patch('utility.get_deb_arch', return_value='amd65',
                                autospec=True))

    def test_fetch_juju_binary(self):
        key = mock_package_key(275)
        filename = get_key_filename(key)
        bucket = FakeBucket([key])

        def extract(package, out_dir):
            parent = os.path.join(out_dir, 'bin')
            os.makedirs(parent)
            open(os.path.join(parent, 'juju'), 'w')

        with temp_dir() as workspace:
            with patch('jujuci.extract_deb', autospec=True,
                       side_effect=extract) as ed_mock:
                extracted = fetch_juju_binary(bucket, 275, workspace)
        local_deb = os.path.join(workspace, filename)
        key.get_contents_to_filename.assert_called_once_with(local_deb)
        eb_dir = os.path.join(workspace, 'extracted-bin')
        ed_mock.assert_called_once_with(local_deb, eb_dir)
        self.assertEqual(os.path.join(eb_dir, 'bin', 'juju'), extracted)


class TestFetchFiles(StrictTestCase):

    def setUp(self):
        use_context(self, patch('utility.get_deb_arch', return_value='amd65',
                                autospec=True))

    def test_fetch_files(self):
        key = mock_key(275, 'job-foo', 27, 'file-pattern')
        bucket = FakeBucket([key])
        with temp_dir() as workspace:
            downloaded = fetch_files(bucket, 275, 'job-foo', 'file-pat+ern',
                                     workspace)
        local_file = os.path.join(workspace, 'file-pattern')
        key.get_contents_to_filename.assert_called_once_with(local_file)
        key_copy = os.path.join(workspace, local_file)
        self.assertEqual([key_copy], downloaded)


class FakeKey:

    def __init__(self, revision_build, job, build, file_path):
        job_path = get_job_path(revision_build, job)
        self.name = '{}/build-{}/{}'.format(job_path, build, file_path)


class FakeBucket:

    def __init__(self, keys):
        self.keys = keys

    def list(self, prefix):
        return [key for key in self.keys if key.name.startswith(prefix)]


class TestFindFileKeys(StrictTestCase):

    def test_find_file_keys(self):
        key = FakeKey(275, 'job-foo', 27, 'file-pattern')
        bucket = FakeBucket([key])
        filtered = find_file_keys(bucket, 275, 'job-foo', 'file-pat+ern')
        self.assertEqual([key], filtered)

    def test_matches_pattern(self):
        match_key = FakeKey(275, 'job-foo', 27, 'file-pattern')
        wrong_name = FakeKey(275, 'job-foo', 27, 'file-pat+ern')
        wrong_job = FakeKey(275, 'job.foo', 27, 'file-pattern')
        wrong_rb = FakeKey(276, 'job-foo', 27, 'file-pattern')
        keys = [match_key, wrong_name, wrong_job, wrong_rb]
        bucket = FakeBucket(keys)
        filtered = find_file_keys(
            bucket, 275, 'job-foo', 'file-pat+ern')
        self.assertEqual([match_key], filtered)

    def test_uses_latest_build(self):
        bucket = FakeBucket([FakeKey(275, 'job-foo', 1, 'file-pattern')])
        filtered = find_file_keys(bucket, 275, 'job-foo', 'file-pat+ern')
        self.assertEqual([bucket.keys[0]], filtered)

        bucket.keys.append(FakeKey(275, 'job-foo', 2, 'non-match'))
        filtered = find_file_keys(bucket, 275, 'job-foo', 'file-pat+ern')
        self.assertEqual([], filtered)

        bucket.keys.append(FakeKey(275, 'job-foo', 2, 'file-pattern'))
        filtered = find_file_keys(bucket, 275, 'job-foo', 'file-pat+ern')
        self.assertEqual([bucket.keys[2]], filtered)

    def test_pattern_is_path(self):
        match_key = FakeKey(275, 'job-foo', 27, 'dir/file')
        bucket = FakeBucket([match_key])
        filtered = find_file_keys(bucket, 275, 'job-foo', 'dir/file')
        self.assertEqual([match_key], filtered)


class TestMain(StrictTestCase):

    @contextmanager
    def temp_credentials(self):
        with NamedTemporaryFile() as temp_file:
            temp_file.write(dedent("""\
                [default]
                access_key = fake_username
                secret_key = fake_pass
                """))
            temp_file.flush()
            yield temp_file.name

    def setUp(self):
        use_context(self, stdout_guard())

    def test_main_args_get_juju_bin(self):
        stdout = StringIO()
        with self.temp_credentials() as config_file:
            with patch('sys.argv', [
                    'foo', 'get-juju-bin', '28', 'bar-workspace',
                    '--config', config_file]):
                with patch('s3ci.S3Connection', autospec=True) as s3c_mock:
                    with patch('s3ci.fetch_juju_binary', autospec=True,
                               return_value='gjb') as gbj_mock:
                        with patch('sys.stdout', stdout):
                            main()
        s3c_mock.assert_called_once_with('fake_username', 'fake_pass')
        gb_mock = s3c_mock.return_value.get_bucket
        gb_mock.assert_called_once_with(JUJU_QA_DATA)
        gbj_mock.assert_called_once_with(gb_mock.return_value, 28,
                                         'bar-workspace')
        self.assertEqual('gjb\n', stdout.getvalue())

    def test_main_args_get(self):
        stdout = StringIO()
        with self.temp_credentials() as config_file:
            with patch('sys.argv', [
                    'foo', 'get', '28', 'foo-job', 'bar-file', 'bar-workspace',
                    '--config', config_file]):
                with patch('s3ci.S3Connection', autospec=True) as s3c_mock:
                    with patch('s3ci.fetch_files', autospec=True,
                               return_value=['ff', 'gg']) as ff_mock:
                        with patch('sys.stdout', stdout):
                            main()
        s3c_mock.assert_called_once_with('fake_username', 'fake_pass')
        gb_mock = s3c_mock.return_value.get_bucket
        gb_mock.assert_called_once_with(JUJU_QA_DATA)
        ff_mock.assert_called_once_with(gb_mock.return_value, 28,
                                        'foo-job', 'bar-file', 'bar-workspace')
        self.assertEqual('ff\ngg\n', stdout.getvalue())
