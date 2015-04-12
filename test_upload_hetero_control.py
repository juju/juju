from unittest import TestCase
from upload_hetero_control import (
    JenkinsBuild,
    S3,
    HUploader,
    get_s3_access,
    add_credential_args,
    get_credentials,
)
from ConfigParser import NoOptionError
from argparse import Namespace, ArgumentParser
from collections import namedtuple
from mock import patch, MagicMock
import json
from StringIO import StringIO
from jujuci import (
    JENKINS_URL,
    CredentialsMissing,
    Credentials,
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
        jenkins_cxt = patch('upload_hetero_control.Jenkins', autospec=True)
        with jenkins_cxt as j_mock:
            j = JenkinsBuild.factory(credentials, JOB_NAME)
            self.assertIs(type(j), JenkinsBuild)
        j_mock.assert_called_once_with(
            JENKINS_URL, credentials.user, credentials.password)
        self.assertEqual(j_mock.return_value.get_build_info.call_count, 0)

    def test_factory_with_build_number(self):
        credentials = fake_credentials()
        jenkins_cxt = patch('upload_hetero_control.Jenkins', autospec=True)
        with jenkins_cxt as j_mock:
            j = JenkinsBuild.factory(credentials, JOB_NAME, BUILD_NUM)
            self.assertIs(type(j), JenkinsBuild)
        j_mock.assert_called_with(
            JENKINS_URL, credentials.user, credentials.password)
        j_mock.return_value.get_build_info.assert_called_with(
            JOB_NAME, BUILD_NUM)

    def test_get_build_info(self):
        credentials = fake_credentials()
        jenkins_mock = MagicMock()
        j = JenkinsBuild(credentials, JOB_NAME, jenkins_mock, None)
        with patch.object(jenkins_mock, 'get_build_info',
                          return_value=BUILD_INFO, autospec=True) as i_mock:
            build_info = j.get_build_info(BUILD_NUM)
            self.assertEqual(build_info, BUILD_INFO)
        i_mock.assert_called_with(JOB_NAME, BUILD_NUM)

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
        u_mock.assert_called_once_with(BUILD_INFO['url'] + 'consoleText',
                                       auth=h_mock.return_value)
        h_mock.assert_called_once_with(credentials[0], credentials[1])

    def test_artifacts(self):
        class Response:
            content = "artifact content"

        credentials = fake_credentials()
        j = JenkinsBuild(credentials, "fake", None, BUILD_INFO)
        with patch('upload_hetero_control.requests.get',
                   return_value=Response, autospec=True) as u_mock:
            with patch('upload_hetero_control.HTTPBasicAuth',
                       return_value=None, autospec=True) as h_mock:
                with patch.object(j, '_get_artifact_url',
                                  autospec=True) as g_mock:
                    for filename, content in j.artifacts():
                        self.assertEqual(content, 'artifact content')
        u_mock.assert_called_once_with(g_mock.return_value, auth=None)
        h_mock.assert_called_once_with(credentials.user, credentials.password)
        g_mock.assert_called_once_with(
            BUILD_INFO['artifacts'][0]['relativePath'])

    def test_get_build_number(self):
        credentials = fake_credentials()
        j = JenkinsBuild(credentials, "fake", None, BUILD_INFO)
        self.assertEqual(j.get_build_number(), BUILD_NUM)

    def test_set_build_number(self):
        credentials = fake_credentials()
        jenkins_cxt = patch('upload_hetero_control.Jenkins', autospec=True)
        build_info = {"number": BUILD_NUM}
        with jenkins_cxt as j_mock:
            j = JenkinsBuild.factory(credentials, JOB_NAME)
            self.assertIs(type(j), JenkinsBuild)
            with patch.object(j, 'get_build_info', return_value=build_info,
                              autospec=True):
                j.set_build_number(BUILD_NUM)
                self.assertEqual(j.build_info, build_info)
        j_mock.assert_called_with(
            JENKINS_URL, credentials.user, credentials.password)


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
        b_mock.new_key.return_value.set_contents_from_string. \
            assert_called_once_with('fake data', headers=None)


class TestHUploader(TestCase):

    def test_factory(self):
        credentials = fake_credentials()
        with patch('upload_hetero_control.S3', autospec=True):
            with patch('upload_hetero_control.JenkinsBuild',
                       autospec=True):
                h = HUploader.factory(credentials, BUILD_NUM)
                self.assertIs(type(h), HUploader)

    def test_upload(self):
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        h = HUploader(s3_mock, jenkins_mock)
        with patch('upload_hetero_control.HUploader.upload_test_results',
                   autospec=True) as s_mock:
            with patch('upload_hetero_control.HUploader.upload_console_log',
                       autospec=True) as u_mock:
                with patch('upload_hetero_control.HUploader.upload_artifacts',
                           autospec=True) as a_mock:
                    h.upload()
        s_mock.assert_called_once_with(h)
        u_mock.assert_called_once_with(h)
        a_mock.assert_called_once_with(h)

    def test_upload_all_test_results_SAVE(self):
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        h = HUploader(s3_mock, jenkins_mock)
        with patch('upload_hetero_control.HUploader.upload',
                   autospec=True) as u_mock:
            with patch.object(jenkins_mock, 'get_latest_build_number',
                              return_value=3, autospec=True) as g_mock:
                h.upload_all_test_results()
        g_mock.assert_called_once_with(h)
        u_mock.assert_called_with(h)
        self.assertEqual(u_mock.call_count, 3)

    def test_upload_test_results(self):
        filename = '1277-result-results.json'
        headers = {"Content-Type": "application/json; charset=utf8"}
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        h = HUploader(s3_mock, jenkins_mock)
        with patch.object(s3_mock, 'store', autopsec=True) as s_mock:
            with patch.object(
                    jenkins_mock, 'get_build_number', return_value=BUILD_NUM,
                    autospec=True) as g_mock:
                with patch.object(
                        jenkins_mock, 'get_build_info', autospec=True,
                        return_value=BUILD_INFO) as b_mock:
                    h.upload_test_results()
            s_mock.assert_called_once_with(
                filename, json.dumps(BUILD_INFO), headers=headers)
            g_mock.assert_called_once_with(h)
            b_mock.assert_called_once_with(h)

    def test_upload_console_log(self):
        filename = '1277-console-consoleText.txt'
        headers = {"Content-Type": "text/plain; charset=utf8"}
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        h = HUploader(s3_mock, jenkins_mock)
        with patch.object(s3_mock, 'store', autopsec=True) as s_mock:
            with patch.object(
                    jenkins_mock, 'get_build_number', return_value=BUILD_NUM,
                    autospec=True) as g_mock:
                with patch.object(
                        jenkins_mock, 'get_console_text', autospec=True,
                        return_value="log text") as b_mock:
                    h.upload_console_log()
            s_mock.assert_called_once_with(
                filename, 'log text', headers=headers)
            g_mock.assert_called_once_with(h)
            b_mock.assert_called_once_with(h)

    def test_upload_artifacts(self):
        filename = '1277-log-filename'
        headers = {"Content-Type": "application/x-gzip"}
        s3_mock = MagicMock()
        jenkins_mock = MagicMock()
        h = HUploader(s3_mock, jenkins_mock)

        def fake_artifacts():
            for x in xrange(1, 4):
                yield "filename", "artifact data"

        with patch.object(s3_mock, 'store', autopsec=True) as s_mock:
            with patch.object(
                    jenkins_mock, 'get_build_number', return_value=BUILD_NUM,
                    autospec=True) as g_mock:
                with patch.object(
                        jenkins_mock, 'artifacts', autospec=True,
                        return_value=fake_artifacts()) as b_mock:
                    h.upload_artifacts()
            s_mock.assert_called_with(filename, 'artifact data',
                                      headers=headers)
            g_mock.assert_called_once_with(h)
            b_mock.assert_called_once_with(h)

    def test_upload_by_env_build_number(self):
        jenkins_mock = MagicMock()
        h = HUploader(None, jenkins_mock)
        with patch('upload_hetero_control.os.getenv',
                   return_value='399993', autospec=True) as g_mock:
            with patch.object(jenkins_mock, 'set_build_number',
                              autospec=True) as s_mock:
                with patch.object(h, 'upload', autospec=True) as u_mock:
                    h.upload_by_env_build_number()
        g_mock.assert_called_once_with('BUILD_NUMBER')
        s_mock.assert_called_once_with('399993')
        u_mock.assert_called_once_with()

    def test_upload_by_env_build_number__no_build_number(self):
        jenkins_mock = MagicMock()
        h = HUploader(None, jenkins_mock)
        with patch('upload_hetero_control.os.getenv',
                   return_value=None, autospec=True):
            with patch.object(jenkins_mock, 'set_build_number', autospec=True):
                with patch.object(h, 'upload', autospec=True):
                    with self.assertRaises(ValueError):
                        h.upload_by_env_build_number()

    def test_upload_by_env_build_number__not_number(self):
        jenkins_mock = MagicMock()
        h = HUploader(None, jenkins_mock)
        with patch('upload_hetero_control.os.getenv',
                   return_value="1Hello2", autospec=True):
            with patch.object(jenkins_mock, 'set_build_number', autospec=True):
                with patch.object(h, 'upload', autospec=True):
                    with self.assertRaises(ValueError):
                        h.upload_by_env_build_number()


class OtherTests(TestCase):

    def test_get_s3_access(self):
        path = '/u/home'
        relative_path = '/cloud-city/juju-qa.s3cfg'
        with patch('upload_hetero_control.os.getenv',
                   return_value=path, autospec=True) as j_mock:
            with patch('__builtin__.open',
                       return_value=s3cfg(), autospec=True) as o_mock:
                access_key, secret_key = get_s3_access()
                self.assertEqual(access_key, "fake_username")
                self.assertEqual(secret_key, "fake_pass")
        j_mock.assert_called_once_with('HOME')
        o_mock.assert_called_once_with(path + relative_path)

    def test_get_s3_access_no_access_key(self):
        path = '/u/home'
        relative_path = '/cloud-city/juju-qa.s3cfg'
        with patch('upload_hetero_control.os.getenv',
                   return_value=path, autospec=True) as j_mock:
            with patch('__builtin__.open',
                       return_value=s3cfg_no_access_key(),
                       autospec=True) as o_mock:
                with self.assertRaises(NoOptionError):
                    access_key, secret_key = get_s3_access()
                    self.assertEqual(secret_key, "fake_pass")
        j_mock.assert_called_once_with('HOME')
        o_mock.assert_called_once_with(path + relative_path)

    def test_get_s3_access_no_secret_key(self):
        path = '/u/home'
        relative_path = '/cloud-city/juju-qa.s3cfg'
        with patch('upload_hetero_control.os.getenv',
                   return_value=path, autospec=True) as j_mock:
            with patch('__builtin__.open',
                       return_value=s3cfg_no_secret_key(),
                       autospec=True) as o_mock:
                with self.assertRaises(NoOptionError):
                    get_s3_access()
        j_mock.assert_called_once_with('HOME')
        o_mock.assert_called_once_with(path + relative_path)

    def test_get_credentials(self):
        self.assertEqual(
            get_credentials(Namespace(user='jrandom', password='password1')),
            Credentials('jrandom', 'password1'))

    def test_get_credentials_no_user(self):
        self.assertIs(get_credentials(Namespace()), None)

    def test_get_credentials_no_value(self):
        with self.assertRaisesRegexp(
                CredentialsMissing,
                'Jenkins username and/or password not supplied.'):
            get_credentials(Namespace(user=None, password='password1'))
        with self.assertRaisesRegexp(
                CredentialsMissing,
                'Jenkins username and/or password not supplied.'):
            get_credentials(Namespace(user='jrandom', password=None))

    def test_add_credential_args(self):
        parser = ArgumentParser()
        parser.add_argument("--test", help="Testing", default="env")
        self.assertIsNone(parser.get_default('user'))
        self.assertIsNone(parser.get_default('password'))
        with patch('upload_hetero_control.os.environ.get',
                   return_value='fake', autospec=True) as g_mock:
            add_credential_args(parser)
            self.assertEqual(parser.get_default('user'), 'fake')
            self.assertEqual(parser.get_default('password'), 'fake')
            self.assertIsNone(parser.get_default('fake_arg'))
        g_mock.assert_any_call('JENKINS_USER')
        g_mock.assert_any_call('JENKINS_PASSWORD')
        self.assertEqual(g_mock.call_count, 2)


FAKE_CREDENTIALS = namedtuple('Credentials', ['user', 'password'])


def fake_credentials():
    return FAKE_CREDENTIALS('fake_username', 'fake_pass')


def s3cfg():
    data = """
[default]
access_key = fake_username
secret_key = fake_pass
"""
    return StringIO(data)


def s3cfg_no_access_key():
    data = """
[default]
secret_key = fake_pass
"""
    return StringIO(data)


def s3cfg_no_secret_key():
    data = """
[default]
access_key = fake_username
"""
    return StringIO(data)
