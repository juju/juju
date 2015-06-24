from ConfigParser import NoOptionError
import json
from unittest import TestCase

from mock import patch, MagicMock, call
from tempfile import NamedTemporaryFile

from jujuci import (
    Credentials,
    JENKINS_URL,
)
from upload_hetero_control import (
    get_s3_access,
    HUploader,
    JenkinsBuild,
    S3,
)


__metaclass__ = type


JOB_NAME = 'compatibility-control'
BUILD_NUM = 1277
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
        with patch('upload_hetero_control.get_build_data',
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
        with patch('upload_hetero_control.get_build_data',
                   autospec=True, return_value=BUILD_INFO) as gbd_mock:
            j = JenkinsBuild.factory(credentials, JOB_NAME, BUILD_NUM)
        self.assertIs(type(j), JenkinsBuild)
        self.assertEqual(j.build_info, BUILD_INFO)
        gbd_mock.assert_called_once_with(
            JENKINS_URL, credentials, JOB_NAME, BUILD_NUM)

    def test_get_build_info(self):
        credentials = fake_credentials()
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, None)
        with patch('upload_hetero_control.get_build_data', autospec=True,
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
        with patch('upload_hetero_control.requests.get',
                   return_value=Response, autospec=True) as u_mock:
            with patch('upload_hetero_control.HTTPBasicAuth',
                       autospec=True) as h_mock:
                text = j.get_console_text()
                self.assertEqual(text, 'console content')
        u_mock.assert_called_once_with(
            BUILD_INFO['url'] + 'consoleText', auth=h_mock.return_value)
        h_mock.assert_called_once_with(credentials[0], credentials[1])

    def test_get_last_completed_build_number(self):
        last_build = {"lastCompletedBuild": {"number": BUILD_NUM}}
        credentials = fake_credentials()
        with patch("upload_hetero_control.get_job_data", autospec=True,
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
        with patch('upload_hetero_control.requests.get',
                   return_value=Response, autospec=True) as u_mock:
            with patch('upload_hetero_control.HTTPBasicAuth',
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
        with patch('upload_hetero_control.get_build_data', autospec=True,
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
        with patch('upload_hetero_control.get_build_data', autospec=True,
                   return_value=BUILD_INFO) as gbd_mock:
            build_status = j.is_build_completed()
        self.assertIs(build_status, True)
        self.assertEqual(gbd_mock.mock_calls, create_build_data_calls())

    def test_is_build_completed_return_false(self):
        credentials = fake_credentials()
        build_info = json.loads('{"building": true}')
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, BUILD_INFO)
        with patch('upload_hetero_control.get_build_data', autospec=True,
                   return_value=build_info) as gbd_mock:
            build_status = j.is_build_completed()
        self.assertIs(build_status, False)
        self.assertEqual(gbd_mock.mock_calls, create_build_data_calls())


class TestS3(TestCase):
    def test_factory(self):
        cred = ('fake_user', 'fake_pass')
        s3conn_cxt = patch(
            'upload_hetero_control.S3Connection', autospec=True)
        with s3conn_cxt as j_mock:
            with patch('upload_hetero_control.get_s3_access',
                       return_value=cred, autospec=True) as g_mock:
                s3 = S3.factory()
                self.assertIs(type(s3), S3)
        g_mock.assert_called_once_with()
        j_mock.assert_called_once_with(cred[0], cred[1])

    def test_store(self):
        b_mock = MagicMock()
        s3 = S3('/comp-test', 'fake', 'fake', None, b_mock)
        status = s3.store('fake filename', 'fake data')
        self.assertTrue(status, True)
        (b_mock.new_key.return_value.set_contents_from_string.
            assert_called_once_with('fake data', headers=None))


class TestHUploader(TestCase):

    def test_factory(self):
        credentials = fake_credentials()
        with patch('upload_hetero_control.S3', autospec=True):
            with patch('upload_hetero_control.JenkinsBuild',
                       autospec=True):
                h = HUploader.factory(credentials, BUILD_NUM)
                self.assertIs(type(h), HUploader)

    def test_upload(self):
        filename = '1200-result-results.json'
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_last_completed_build_number.return_value = 1200
        jenkins_mock.get_build_number.return_value = 1200
        jenkins_mock.get_build_info.return_value = {"build_info": "1200"}
        jenkins_mock.get_console_text.return_value = "console text"
        jenkins_mock._create_filename.return_value = filename
        jenkins_mock.artifacts.return_value = fake_artifacts(2)
        h = HUploader(s3_mock, jenkins_mock)
        h.upload()
        self.assertEqual(s3_mock.store.mock_calls, [
            call(filename, json.dumps({"build_info": "1200"}),
                 headers={"Content-Type": "application/json"}),
            call('1200-console-consoleText.txt', 'console text',
                 headers={"Content-Type": "text/plain; charset=utf8"}),
            call('1200-log-filename', 'artifact data 1',
                 headers={"Content-Type": "application/octet-stream"})])

    def test_upload_all_test_results(self):
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_last_completed_build_number.return_value = 3
        jenkins_mock.get_build_info.return_value = BUILD_INFO
        h = HUploader(s3_mock, jenkins_mock)
        h.upload_all_test_results()
        self.assertEqual(jenkins_mock.set_build_number.mock_calls,
                         [call(1), call(2), call(3)])

    def test_upload_test_results(self):
        filename = '1277-result-results.json'
        headers = {"Content-Type": "application/json"}
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_build_info.return_value = BUILD_INFO
        jenkins_mock.get_build_number.return_value = BUILD_NUM
        h = HUploader(s3_mock, jenkins_mock)
        h.upload_test_results()
        s3_mock.store.assert_called_once_with(
            filename, json.dumps(jenkins_mock.get_build_info.return_value),
            headers=headers)

    def test_upload_console_log(self):
        filename = '1277-console-consoleText.txt'
        headers = {"Content-Type": "text/plain; charset=utf8"}
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_build_number.return_value = BUILD_NUM
        jenkins_mock.get_console_text.return_value = "log text"
        h = HUploader(s3_mock, jenkins_mock)
        h.upload_console_log()
        s3_mock.store.assert_called_once_with(
            filename, 'log text', headers=headers)
        jenkins_mock.get_console_text.assert_called_once_with()

    def test_upload_artifacts(self):
        filename = '1277-log-filename'
        headers = {"Content-Type": "application/octet-stream"}
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        jenkins_mock.get_build_number.return_value = BUILD_NUM
        jenkins_mock.artifacts.return_value = fake_artifacts(4)
        h = HUploader(s3_mock, jenkins_mock)
        h.upload_artifacts()
        calls = [call(filename, 'artifact data 1', headers=headers),
                 call(filename, 'artifact data 2', headers=headers),
                 call(filename, 'artifact data 3', headers=headers)]
        self.assertEqual(s3_mock.store.mock_calls, calls)
        jenkins_mock.artifacts.assert_called_once_with()

    def test_upload_by_build_number(self):
        credentials = fake_credentials()
        build_info = {"number": 9988, 'building': False}
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, BUILD_INFO)
        uploader = HUploader(None, j)
        with patch('upload_hetero_control.os.getenv',
                   return_value=9988, autospec=True) as g_mock:
            with patch('upload_hetero_control.get_build_data', autospec=True,
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
        h = HUploader(None, jenkins_mock)
        with patch('upload_hetero_control.os.getenv',
                   return_value=None, autospec=True):
            with self.assertRaisesRegexp(
                    ValueError, 'Build number is not set'):
                h.upload_by_build_number()

    def test_upload_by_build_number_timeout(self):
        credentials = fake_credentials()
        build_info = {"number": 9988, 'building': True}
        j = JenkinsBuild(credentials, JOB_NAME, JENKINS_URL, BUILD_INFO)
        uploader = HUploader(None, j)
        with patch('upload_hetero_control.get_build_data', autospec=True,
                   return_value=build_info) as gbd_mock:
            with self.assertRaisesRegexp(
                    Exception, "Build fails to complete: 9988"):
                uploader.upload_by_build_number(
                    build_number=9988, pause_time_in_seconds=.1,
                    total_timeout_in_seconds=.1)
        self.assertEqual(
            gbd_mock.mock_calls,
            create_build_data_calls(build_num=9988, calls=2))

    def test_last_completed_test_results(self):
        class Response:
            text = "console content"
        build_info = {"artifacts": [], 'url': 'fake', "number": BUILD_NUM}
        last_build = {"lastCompletedBuild": {"number": BUILD_NUM}}
        cred = Credentials('joe', 'password')
        jenkins_build = JenkinsBuild(cred, None, None, build_info)
        s3_mock = MagicMock()
        h = HUploader(s3_mock, jenkins_build)
        with patch("upload_hetero_control.get_job_data", autospec=True,
                   return_value=last_build) as gjd_mock:
            with patch("upload_hetero_control.get_build_data", autospec=True,
                       return_value=build_info) as gbd_mock:
                with patch('upload_hetero_control.requests.get', autospec=True,
                           return_value=Response):
                    h.upload_last_completed_test_result()
                    self.assertEqual(
                        h.jenkins_build.get_last_completed_build_number(),
                        BUILD_NUM)
        self.assertEqual(s3_mock.store.mock_calls, [
            call('1277-result-results.json', json.dumps(build_info),
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


class OtherTests(TestCase):

    def test_get_s3_access(self):
        path = '/u/home'
        relative_path = 'cloud-city/juju-qa.s3cfg'
        with NamedTemporaryFile() as temp_file:
                temp_file.write(s3cfg())
                temp_file.flush()
                with patch(
                        'upload_hetero_control.os.path.join', autospec=True,
                        return_value=temp_file.name) as j_mock:
                    with patch(
                            'upload_hetero_control.os.getenv', autospec=True,
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
                with patch('upload_hetero_control.os.path.join', autospec=True,
                           return_value=temp_file.name) as j_mock:
                    with patch(
                            'upload_hetero_control.os.getenv', autospec=True,
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
                        'upload_hetero_control.os.path.join', autospec=True,
                        return_value=temp_file.name) as j_mock:
                    with patch(
                            'upload_hetero_control.os.getenv', autospec=True,
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
