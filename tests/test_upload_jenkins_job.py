from argparse import Namespace
from ConfigParser import NoOptionError
import json
from time import sleep
from unittest import TestCase

from mock import patch, MagicMock, call
from tempfile import NamedTemporaryFile

from jujuci import (
    Credentials,
    JENKINS_URL,
)
from upload_jenkins_job import (
    get_args,
    get_s3_access,
    S3Uploader,
    JenkinsBuild,
    S3,
)


__metaclass__ = type


JOB_NAME = 'compatibility-control'
BUILD_NUM = 1277
BUCKET = 'juju-qa'
DIRECTORY = 'foo'

BUILD_INFO = json.loads(
    json.dumps(
        {'building': False,
         'artifacts': [{"relativePath": "logs/all-machines.log.gz",
                        "displayPath": "all-machines.log.gz",
                        "fileName": "all-machines.log.gz"
                        }],
         'timestamp': 1411053288000,
         'number': BUILD_NUM,
         'result': 'SUCCESS',
         'duration': 176239,
         "build_number": 1024,
         "new_to_old": False,
         "candidate": "1.21",
         "old_version": "1.20.5",
         "url": 'http://juju-ci.vapour.ws:8080/job/compatibility-control/1277/'
         }))


class TestJenkinsBuild(TestCase):

    def test_factory(self):
        credentials = fake_credentials()
        with patch('upload_jenkins_job.get_build_data',
                   autospec=True) as gbd_mock:
            j = JenkinsBuild.factory(credentials, JOB_NAME)
        self.assertIs(type(j), JenkinsBuild)
        self.assertEqual(j.job_name, JOB_NAME)
        self.assertEqual(j.credentials, credentials)
        self.assertEqual(j.build_info, None)
        self.assertEqual(j.jenkins_url, JENKINS_URL)
        self.assertEqual(gbd_mock.call_count, 0)

    def test_factory_with_build_number(self):
        credentials = fake_credentials()
        with patch('upload_jenkins_job.get_build_data',
                   autospec=True, return_value=BUILD_INFO) as gbd_mock:
            j = JenkinsBuild.factory(credentials, JOB_NAME, BUILD_NUM)
        self.assertIs(type(j), JenkinsBuild)
        self.assertEqual(j.build_info, BUILD_INFO)
        gbd_mock.assert_called_once_with(
            JENKINS_URL, credentials, JOB_NAME, BUILD_NUM)

    def test_get_build_info(self):
        credentials = fake_credentials()
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, None)
        with patch('upload_jenkins_job.get_build_data', autospec=True,
                   return_value=BUILD_INFO) as gbd_mock:
            build_info = j.get_build_info(BUILD_NUM)
        self.assertEqual(build_info, BUILD_INFO)
        gbd_mock.assert_called_once_with(
            JENKINS_URL, credentials, JOB_NAME, BUILD_NUM)

    def test_result(self):
        credentials = fake_credentials()
        j = JenkinsBuild(credentials, JOB_NAME, None, BUILD_INFO)
        self.assertEqual(j.result, BUILD_INFO['result'])

    def test_console_text(self):
        class Response:
            text = "console content"

        credentials = fake_credentials()
        j = JenkinsBuild(credentials, "fake", None, BUILD_INFO)
        with patch('upload_jenkins_job.requests.get',
                   return_value=Response, autospec=True) as u_mock:
            with patch('upload_jenkins_job.HTTPBasicAuth',
                       autospec=True) as h_mock:
                text = j.get_console_text()
                self.assertEqual(text, 'console content')
        u_mock.assert_called_once_with(
            BUILD_INFO['url'] + 'consoleText', auth=h_mock.return_value)
        h_mock.assert_called_once_with(credentials[0], credentials[1])

    def test_get_last_completed_build_number(self):
        last_build = {"lastCompletedBuild": {"number": BUILD_NUM}}
        credentials = fake_credentials()
        with patch("upload_jenkins_job.get_job_data", autospec=True,
                   return_value=last_build) as gjd_mock:
            j = JenkinsBuild(credentials, JOB_NAME, None, BUILD_INFO)
            last_build_number = j.get_last_completed_build_number()
        self.assertEqual(last_build_number, BUILD_NUM)
        gjd_mock.assert_called_once_with(None, credentials, JOB_NAME)

    def test_artifacts(self):
        class Response:
            content = "artifact content"

        credentials = fake_credentials()
        j = JenkinsBuild(credentials, "fake", None, BUILD_INFO)
        expected = BUILD_INFO['url'] + 'artifact/' + 'logs/all-machines.log.gz'
        with patch('upload_jenkins_job.requests.get',
                   return_value=Response, autospec=True) as u_mock:
            with patch('upload_jenkins_job.HTTPBasicAuth',
                       return_value=None, autospec=True) as h_mock:
                    for filename, content in j.artifacts():
                        self.assertEqual(content, 'artifact content')
        u_mock.assert_called_once_with(
            expected, auth=h_mock.return_value)
        h_mock.assert_called_once_with(credentials.user, credentials.password)

    def test_get_build_number(self):
        credentials = fake_credentials()
        j = JenkinsBuild(credentials, "fake", None, BUILD_INFO)
        self.assertEqual(j.get_build_number(), BUILD_NUM)

    def test_set_build_number(self):
        credentials = fake_credentials()
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, None)
        with patch('upload_jenkins_job.get_build_data', autospec=True,
                   return_value=BUILD_INFO) as gbd_mock:
            j.set_build_number(BUILD_NUM)
            build_info = j.get_build_info(BUILD_NUM)
        self.assertEqual(build_info, BUILD_INFO)
        gbd_mock.assert_called_with(
            JENKINS_URL, credentials, JOB_NAME, BUILD_NUM)
        self.assertEqual(2, gbd_mock.call_count)

    def test_is_build_completed(self):
        credentials = fake_credentials()
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, BUILD_INFO)
        with patch('upload_jenkins_job.get_build_data', autospec=True,
                   return_value=BUILD_INFO) as gbd_mock:
            build_status = j.is_build_completed()
        self.assertIs(build_status, True)
        self.assertEqual(gbd_mock.mock_calls, create_build_data_calls())

    def test_is_build_completed_return_false(self):
        credentials = fake_credentials()
        build_info = json.loads('{"building": true}')
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, BUILD_INFO)
        with patch('upload_jenkins_job.get_build_data', autospec=True,
                   return_value=build_info) as gbd_mock:
            build_status = j.is_build_completed()
        self.assertIs(build_status, False)
        self.assertEqual(gbd_mock.mock_calls, create_build_data_calls())


class TestS3(TestCase):
    def test_factory(self):
        cred = ('fake_user', 'fake_pass')
        s3conn_cxt = patch(
            'upload_jenkins_job.S3Connection', autospec=True)
        with s3conn_cxt as j_mock:
            with patch('upload_jenkins_job.get_s3_access',
                       return_value=cred, autospec=True) as g_mock:
                s3 = S3.factory('buck', 'dir')
                self.assertIs(type(s3), S3)
                self.assertEqual(s3.dir, 'dir')
                self.assertEqual(('buck',), j_mock.mock_calls[1][1])
        g_mock.assert_called_once_with()
        j_mock.assert_called_once_with(cred[0], cred[1])

    def test_store(self):
        b_mock = MagicMock()
        s3 = S3('/comp-test', 'fake', 'fake', None, b_mock)
        status = s3.store('fake filename', 'fake data')
        self.assertTrue(status, True)
        (b_mock.new_key.return_value.set_contents_from_string.
            assert_called_once_with('fake data', headers=None))


class TestS3Uploader(TestCase):

    def test_factory(self):
        credentials = fake_credentials()
        with patch('upload_jenkins_job.S3', autospec=True) as s_mock:
            with patch('upload_jenkins_job.JenkinsBuild',
                       autospec=True) as j_mock:
                h = S3Uploader.factory(credentials, JOB_NAME, BUILD_NUM,
                                       BUCKET, DIRECTORY)
                self.assertIs(type(h), S3Uploader)
                self.assertEqual((BUCKET, DIRECTORY), s_mock.mock_calls[0][1])
                self.assertEqual(credentials,
                                 j_mock.mock_calls[0][2]['credentials'])
                self.assertEqual(JOB_NAME, j_mock.mock_calls[0][2]['job_name'])
                self.assertEqual(BUILD_NUM,
                                 j_mock.mock_calls[0][2]['build_number'])

    def test_upload(self):
        filename, s3_mock, jenkins_mock = (
            self._make_upload(file_prefix=BUILD_NUM))
        h = S3Uploader(s3_mock, jenkins_mock)
        h.upload()
        self.assertEqual(s3_mock.store.mock_calls, [
            call(filename, json.dumps(
                {"build_info": BUILD_NUM, "number": "2222"}, indent=4),
                headers={"Content-Type": "application/json"}),
            call('{}-console-consoleText.txt'.format(BUILD_NUM),
                 'console text',
                 headers={"Content-Type": "text/plain; charset=utf8"}),
            call('{}-log-filename'.format(BUILD_NUM), 'artifact data 1',
                 headers={"Content-Type": "application/octet-stream"})])

    def test_upload_unique_id(self):
        filename, s3_mock, jenkins_mock = self._make_upload(file_prefix='9999')
        h = S3Uploader(s3_mock, jenkins_mock, unique_id='9999')
        h.upload()
        self.assertEqual(s3_mock.store.mock_calls, [
            call(filename,
                 ('{\n    "origin_number": 2222, \n    "build_info": 1277, \n '
                  '   "number": 9999\n}'),
                 headers={"Content-Type": "application/json"}),
            call('9999-console-consoleText.txt', 'console text',
                 headers={"Content-Type": "text/plain; charset=utf8"}),
            call('9999-log-filename', 'artifact data 1',
                 headers={"Content-Type": "application/octet-stream"})])

    def _make_upload(self, file_prefix):
        filename = '{}-result-results.json'.format(file_prefix)
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_last_completed_build_number.return_value = BUILD_NUM
        jenkins_mock.get_build_number.return_value = BUILD_NUM
        jenkins_mock.get_build_info.return_value = {"build_info": BUILD_NUM,
                                                    "number": "2222"}
        jenkins_mock.get_console_text.return_value = "console text"
        jenkins_mock._create_filename.return_value = filename
        jenkins_mock.artifacts.return_value = fake_artifacts(2)
        return filename, s3_mock, jenkins_mock

    def test_upload_all_test_results(self):
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_last_completed_build_number.return_value = 3
        jenkins_mock.get_build_info.return_value = BUILD_INFO
        h = S3Uploader(s3_mock, jenkins_mock)
        h.upload_all_test_results()
        self.assertEqual(jenkins_mock.set_build_number.mock_calls,
                         [call(1), call(2), call(3)])

    def test_upload_test_results(self):
        filename, headers, s3_mock, jenkins_mock = (
            self._make_upload_test_results(file_prefix=BUILD_NUM))
        h = S3Uploader(s3_mock, jenkins_mock)
        h.upload_test_results()
        s3_mock.store.assert_called_once_with(
            filename, json.dumps(jenkins_mock.get_build_info.return_value,
                                 indent=4), headers=headers)

    def test_upload_test_results_unique_id(self):
        filename, headers, s3_mock, jenkins_mock = (
            self._make_upload_test_results(file_prefix='9999'))
        h = S3Uploader(s3_mock, jenkins_mock, unique_id='9999')
        h.upload_test_results()
        s3_mock.store.assert_called_once_with(
            filename, json.dumps(jenkins_mock.get_build_info.return_value,
                                 indent=4), headers=headers)

    def _make_upload_test_results(self, file_prefix):
        filename = '{}-result-results.json'.format(file_prefix)
        headers = {"Content-Type": "application/json"}
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_build_info.return_value = BUILD_INFO
        jenkins_mock.get_build_number.return_value = BUILD_NUM
        return filename, headers, s3_mock, jenkins_mock

    def test_upload_console_log_444444(self):
        filename, headers, s3_mock, jenkins_mock = (
            self._make_upload_console_log(file_prefix=BUILD_NUM))
        h = S3Uploader(s3_mock, jenkins_mock)
        h.upload_console_log()
        s3_mock.store.assert_called_once_with(
            filename, 'log text', headers=headers)
        jenkins_mock.get_console_text.assert_called_once_with()

    def test_upload_console_log_unique_id(self):
        filename, headers, s3_mock, jenkins_mock = (
            self._make_upload_console_log(file_prefix='9999'))
        h = S3Uploader(s3_mock, jenkins_mock, unique_id='9999')
        h.upload_console_log()
        s3_mock.store.assert_called_once_with(
            filename, 'log text', headers=headers)
        jenkins_mock.get_console_text.assert_called_once_with()

    def _make_upload_console_log(self, file_prefix):
        filename = '{}-console-consoleText.txt'.format(file_prefix)
        headers = {"Content-Type": "text/plain; charset=utf8"}
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_build_number.return_value = BUILD_NUM
        jenkins_mock.get_console_text.return_value = "log text"
        return filename, headers, s3_mock, jenkins_mock

    def test_make_headers_svg(self):
        headers = S3Uploader.make_headers('/file/path.svg')
        expected = {'Content-Type': 'image/svg+xml'}
        self.assertEqual(headers, expected)

    def test_make_headers_txt_gz(self):
        headers = S3Uploader.make_headers('/file/path.txt.gz')
        expected = {'Content-Type': 'text/plain',
                    'Content-Encoding': 'gzip'}
        self.assertEqual(headers, expected)

    def test_make_headers_log_gz(self):
        headers = S3Uploader.make_headers('path.log.gz')
        expected = {'Content-Type': 'text/plain', 'Content-Encoding': 'gzip'}
        self.assertEqual(headers, expected)

    def test_make_headers_json(self):
        headers = S3Uploader.make_headers('path.json')
        expected = {'Content-Type': 'application/json'}
        self.assertEqual(headers, expected)

    def test_make_headers_yaml(self):
        headers = S3Uploader.make_headers('path.yaml')
        expected = {'Content-Type': 'text/plain'}
        self.assertEqual(headers, expected)

    def test_make_headers_unknown(self):
        headers = S3Uploader.make_headers('path.ab123')
        expected = {'Content-Type': 'application/octet-stream'}
        self.assertEqual(headers, expected)

    def test_upload_artifacts(self):
        filename, headers, s3_mock, jenkins_mock = (
            self._make_upload_artifacts(BUILD_NUM))
        h = S3Uploader(s3_mock, jenkins_mock)
        h.upload_artifacts()
        calls = [call(filename, 'artifact data 1', headers=headers),
                 call(filename, 'artifact data 2', headers=headers),
                 call(filename, 'artifact data 3', headers=headers)]
        self.assertEqual(s3_mock.store.mock_calls, calls)
        jenkins_mock.artifacts.assert_called_once_with()

    def test_upload_artifacts_unique_id(self):
        filename, headers, s3_mock, jenkins_mock = (
            self._make_upload_artifacts('9999'))
        h = S3Uploader(s3_mock, jenkins_mock, unique_id='9999')
        h.upload_artifacts()
        calls = [call(filename, 'artifact data 1', headers=headers),
                 call(filename, 'artifact data 2', headers=headers),
                 call(filename, 'artifact data 3', headers=headers)]
        self.assertEqual(s3_mock.store.mock_calls, calls)
        jenkins_mock.artifacts.assert_called_once_with()

    def test_upload_artifacts_content_type(self):

        def artifacts_fake():
            for filename, i in zip(['foo.log.gz', 'foo.svg'], xrange(1, 3)):
                yield filename, "artifact data {}".format(i)

        _, _, s3_mock, jenkins_mock = (self._make_upload_artifacts(BUILD_NUM))
        jenkins_mock.artifacts.return_value = artifacts_fake()
        h = S3Uploader(s3_mock, jenkins_mock)
        h.upload_artifacts()
        calls = [call('1277-log-foo.log.gz', 'artifact data 1',
                      headers={'Content-Type': 'text/plain',
                               'Content-Encoding': 'gzip'}),
                 call('1277-log-foo.svg', 'artifact data 2',
                      headers={'Content-Type': 'image/svg+xml'})]
        self.assertEqual(s3_mock.store.mock_calls, calls)
        jenkins_mock.artifacts.assert_called_once_with()

    def _make_upload_artifacts(self, file_prefix):
        filename = '{}-log-filename'.format(file_prefix)
        headers = {"Content-Type": "application/octet-stream"}
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_build_number.return_value = BUILD_NUM
        jenkins_mock.artifacts.return_value = fake_artifacts(4)
        return filename, headers, s3_mock, jenkins_mock

    def test_upload_by_build_number(self):
        credentials = fake_credentials()
        build_info = {"number": 9988, 'building': False}
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, BUILD_INFO)
        uploader = S3Uploader(None, j)
        with patch('upload_jenkins_job.os.getenv',
                   return_value=9988, autospec=True) as g_mock:
            with patch('upload_jenkins_job.get_build_data', autospec=True,
                       return_value=build_info) as gbd_mock:
                with patch.object(uploader, 'upload', autospec=True) as u_mock:
                    uploader.upload_by_build_number()
        g_mock.assert_called_once_with('BUILD_NUMBER')
        u_mock.assert_called_once_with()
        self.assertEqual(
            gbd_mock.mock_calls,
            create_build_data_calls(build_num=9988, calls=2))

    def test_upload_by_build_number_no_build_number(self):
        jenkins_mock = MagicMock()
        h = S3Uploader(None, jenkins_mock)
        with patch('upload_jenkins_job.os.getenv',
                   return_value=None, autospec=True):
            with self.assertRaisesRegexp(
                    ValueError, 'Build number is not set'):
                h.upload_by_build_number()

    def test_upload_by_build_number_timeout(self):
        credentials = fake_credentials()
        build_info = {"number": 9988, 'building': True}
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, BUILD_INFO)
        uploader = S3Uploader(None, j)
        with patch('upload_jenkins_job.get_build_data', autospec=True,
                   return_value=build_info) as gbd_mock:
            with self.assertRaisesRegexp(
                    Exception, "Build fails to complete: 9988"):
                uploader.upload_by_build_number(
                    build_number=9988, pause_time=.1, timeout=.1)
        self.assertEqual(
            gbd_mock.mock_calls,
            create_build_data_calls(build_num=9988, calls=2))

    def test_upload_by_build_number_waits(self):
        credentials = fake_credentials()
        build_info = {"number": BUILD_NUM, 'building': True}
        build_info_done = {"number": BUILD_NUM, 'building': False}
        jb = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, BUILD_INFO)
        uploader = S3Uploader(None, jb)
        with patch('upload_jenkins_job.get_build_data', autospec=True,
                   side_effect=[build_info, build_info, build_info_done]) as m:
            with patch.object(uploader, 'upload', autospec=True) as u_mock:
                with patch('upload_jenkins_job.sleep', autospec=True,
                           side_effect=sleep(.1)) as s_mock:
                    uploader.upload_by_build_number(
                        build_number=BUILD_NUM, pause_time=.1, timeout=1)
        self.assertEqual(m.mock_calls, create_build_data_calls(calls=3))
        u_mock.assert_called_once_with()
        s_mock.assert_called_once_with(.1)

    def test_last_completed_test_results(self):
        class Response:
            text = "console content"
        build_info = {"artifacts": [], 'url': 'fake', "number": BUILD_NUM}
        last_build = {"lastCompletedBuild": {"number": BUILD_NUM}}
        cred = Credentials('joe', 'password')
        jenkins_build = JenkinsBuild(cred, None, None, build_info)
        s3_mock = MagicMock()
        h = S3Uploader(s3_mock, jenkins_build)
        with patch("upload_jenkins_job.get_job_data", autospec=True,
                   return_value=last_build) as gjd_mock:
            with patch("upload_jenkins_job.get_build_data", autospec=True,
                       return_value=build_info) as gbd_mock:
                with patch('upload_jenkins_job.requests.get', autospec=True,
                           return_value=Response):
                    h.upload_last_completed_test_result()
                    self.assertEqual(
                        h.jenkins_build.get_last_completed_build_number(),
                        BUILD_NUM)
        self.assertEqual(s3_mock.store.mock_calls, [
            call('1277-result-results.json', json.dumps(build_info, indent=4),
                 headers={"Content-Type": "application/json"}),
            call('1277-console-consoleText.txt', Response.text,
                 headers={"Content-Type": "text/plain; charset=utf8"})
        ])
        self.assertEqual(gjd_mock.mock_calls, [
            call(None, cred, None),
            call(None, cred, None)
        ])
        self.assertEqual(gbd_mock.mock_calls, [
            call(None, cred, None, BUILD_NUM),
            call(None, cred, None, BUILD_NUM)
        ])

    def test_create_file(self):
        filename, s3_mock, jenkins_mock = (
            self._make_upload(file_prefix=BUILD_NUM))
        h = S3Uploader(s3_mock, jenkins_mock)
        filename = h._create_filename("myfile")
        self.assertEqual(filename, "{}-log-myfile".format(BUILD_NUM))

    def test_create_file_console_text(self):
        filename, s3_mock, jenkins_mock = (
            self._make_upload(file_prefix=BUILD_NUM))
        h = S3Uploader(s3_mock, jenkins_mock)
        filename = h._create_filename("consoleText")
        self.assertEqual(
            filename, "{}-console-consoleText.txt".format(BUILD_NUM))

    def test_create_file_result_results(self):
        filename, s3_mock, jenkins_mock = (
            self._make_upload(file_prefix=BUILD_NUM))
        h = S3Uploader(s3_mock, jenkins_mock)
        filename = h._create_filename("result-results.json")
        self.assertEqual(
            filename, "{}-result-results.json".format(BUILD_NUM))

    def test_create_file_no_prefixes(self):
        filename, s3_mock, jenkins_mock = (
            self._make_upload(file_prefix=BUILD_NUM))
        h = S3Uploader(s3_mock, jenkins_mock, no_prefixes=True)
        filename = h._create_filename("myfile")
        self.assertEqual(filename, "myfile")

    def test_create_file_unique_id(self):
        filename, s3_mock, jenkins_mock = (
            self._make_upload(file_prefix=BUILD_NUM))
        h = S3Uploader(s3_mock, jenkins_mock, unique_id='9999')
        filename = h._create_filename("myfile")
        self.assertEqual(filename, "9999-log-myfile")


class OtherTests(TestCase):

    def test_get_args(self):
        args = get_args([JOB_NAME, str(BUILD_NUM), BUCKET, DIRECTORY])
        self.assertEqual(JOB_NAME, args.jenkins_job)
        self.assertEqual(BUILD_NUM, args.build_number)
        self.assertEqual(BUCKET, args.s3_bucket)
        self.assertEqual(DIRECTORY, args.s3_directory)
        self.assertFalse(args.all)
        self.assertFalse(args.latest)
        self.assertIsNone(args.user)
        self.assertIsNone(args.password)

    def test_get_args_all(self):
        args = get_args([JOB_NAME, 'all', BUCKET, DIRECTORY])
        self.assertEqual(JOB_NAME, args.jenkins_job)
        self.assertIsNone(args.build_number)
        self.assertEqual(BUCKET, args.s3_bucket)
        self.assertEqual(DIRECTORY, args.s3_directory)
        self.assertTrue(args.all)
        self.assertFalse(args.latest)
        self.assertIsNone(args.user)
        self.assertIsNone(args.password)

    def test_get_args_latest(self):
        args = get_args([JOB_NAME, 'latest', BUCKET, DIRECTORY])
        self.assertEqual(JOB_NAME, args.jenkins_job)
        self.assertIsNone(args.build_number)
        self.assertEqual(BUCKET, args.s3_bucket)
        self.assertEqual(DIRECTORY, args.s3_directory)
        self.assertFalse(args.all)
        self.assertTrue(args.latest)
        self.assertIsNone(args.user)
        self.assertIsNone(args.password)

    def test_get_args_with_credentials(self):
        args = get_args(['--user', 'me', '--password', 'passwd', JOB_NAME,
                        str(BUILD_NUM), BUCKET, DIRECTORY])
        self.assertEqual(JOB_NAME, args.jenkins_job)
        self.assertEqual(BUILD_NUM, args.build_number)
        self.assertEqual(BUCKET, args.s3_bucket)
        self.assertEqual(DIRECTORY, args.s3_directory)
        self.assertFalse(args.all)
        self.assertFalse(args.latest)
        self.assertEqual(args.user, 'me')
        self.assertEqual(args.password, 'passwd')

    def test_get_args_default(self):
        args = get_args([JOB_NAME, str(BUILD_NUM), BUCKET, DIRECTORY])
        self.assertEqual(args, Namespace(
            all=False, build_number=1277, jenkins_job=JOB_NAME, latest=False,
            password=None, s3_bucket=BUCKET, s3_directory=DIRECTORY,
            unique_id=None, user=None, no_prefixes=False))

    def test_get_s3_access(self):
        path = '/u/home'
        relative_path = 'cloud-city/juju-qa.s3cfg'
        with NamedTemporaryFile() as temp_file:
                temp_file.write(s3cfg())
                temp_file.flush()
                with patch(
                        'upload_jenkins_job.os.path.join', autospec=True,
                        return_value=temp_file.name) as j_mock:
                    with patch(
                            'upload_jenkins_job.os.getenv', autospec=True,
                            return_value=path) as g_mock:
                        access_key, secret_key = get_s3_access()
                        self.assertEqual(access_key, "fake_username")
                        self.assertEqual(secret_key, "fake_pass")
        j_mock.assert_called_once_with(path, relative_path)
        g_mock.assert_called_once_with('HOME')

    def test_get_s3_access_no_access_key(self):
        path = '/u/home'
        relative_path = 'cloud-city/juju-qa.s3cfg'
        with NamedTemporaryFile() as temp_file:
                temp_file.write(s3cfg_no_access_key())
                temp_file.flush()
                with patch('upload_jenkins_job.os.path.join', autospec=True,
                           return_value=temp_file.name) as j_mock:
                    with patch(
                            'upload_jenkins_job.os.getenv', autospec=True,
                            return_value=path) as g_mock:
                        with self.assertRaisesRegexp(
                                NoOptionError,
                                "No option 'access_key' in section: "
                                "'default'"):
                            get_s3_access()
        j_mock.assert_called_once_with(path, relative_path)
        g_mock.assert_called_once_with('HOME')

    def test_get_s3_access_no_secret_key(self):
        path = '/u/home'
        relative_path = 'cloud-city/juju-qa.s3cfg'
        with NamedTemporaryFile() as temp_file:
                temp_file.write(s3cfg_no_secret_key())
                temp_file.flush()
                with patch(
                        'upload_jenkins_job.os.path.join', autospec=True,
                        return_value=temp_file.name) as j_mock:
                    with patch(
                            'upload_jenkins_job.os.getenv', autospec=True,
                            return_value=path) as g_mock:
                        with self.assertRaisesRegexp(
                                NoOptionError,
                                "No option 'secret_key' in section: "
                                "'default'"):
                            get_s3_access()
        j_mock.assert_called_once_with(path, relative_path)
        g_mock.assert_called_once_with('HOME')


def create_build_data_calls(
        url=JENKINS_URL, cred=None, job_name=JOB_NAME,
        build_num=BUILD_NUM, calls=1):
    cred = Credentials('fake_username', 'fake_pass') if not cred else cred
    return [call(url, cred, job_name, build_num) for _ in xrange(calls)]


def fake_credentials():
    return Credentials('fake_username', 'fake_pass')


def s3cfg():
    return """[default]
access_key = fake_username
secret_key = fake_pass
"""


def s3cfg_no_access_key():
    return """
[default]
secret_key = fake_pass
"""


def s3cfg_no_secret_key():
    return """
[default]
access_key = fake_username
"""


def fake_artifacts(max=4):
    for x in range(1, max):
        yield "filename", "artifact data %s" % x
