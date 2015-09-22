from argparse import Namespace
import os
from tempfile import NamedTemporaryFile
from unittest import TestCase

from mock import patch, call

from run_download_juju import (
    create_workspace_yaml,
    get_revisions,
    parse_args,
    main
)
from jujupy import ensure_dir
from test_schedule_hetero_control import make_build_var_file
from test_utility import parse_error
from utility import temp_dir


class TestGetCandidateInfo(TestCase):

    def test_get_candidate_info(self):
        with temp_dir() as dir_path:
            candidate_dir = os.path.join(dir_path, 'candidate')
            ensure_dir(candidate_dir)
            for ver, rev in [['1.24.3', '2870'], ['1.24.5', '2999']]:
                ver_dir = os.path.join(candidate_dir, ver)
                ensure_dir(ver_dir)
                make_build_var_file(ver_dir, version=ver, revision_build=rev)
            rev = get_revisions(dir_path)
        self.assertEqual(rev, ['2870', '2999'])

    def test_create_workspace_yaml(self):
        with NamedTemporaryFile() as temp_file:
            create_workspace_yaml("/my/home", "/script/path", temp_file)
            with open(temp_file.name) as yaml_file:
                content = yaml_file.read()
        self.assertEqual(content, self.expected())

    def test_create_workspace_yaml_with_rev(self):
        with NamedTemporaryFile() as temp_file:
            create_workspace_yaml(
                "/my/home", "/script/path", temp_file, ['2870', '2999'])
            with open(temp_file.name) as yaml_file:
                content = yaml_file.read()
        self.assertEqual(content, self.expected('-c 2870 2999'))

    def test_parse_args_default(self):
        org = os.environ.get('JUJU_HOME')
        os.environ['JUJU_HOME'] = '/juju/home'
        args = parse_args([])
        expected_args = Namespace(
            osx_host='jenkins@osx-slave.vapour.ws',
            win_host='Administrator@win-slave.vapour.ws',
            juju_home='/juju/home')
        self.assertEqual(args, expected_args)
        set_env('JUJU_HOME', org)

    def test_parse_args(self):
        args = parse_args(['-o', 'j@my-osx-host', '-w', 'j@my-win-host',
                           '-j' '/juju/home'])
        expected_args = Namespace(
            osx_host='j@my-osx-host', win_host='j@my-win-host',
            juju_home='/juju/home')
        self.assertEqual(args, expected_args)

    def test_parse_args_juju_home_unset(self):
        org = del_env('JUJU_HOME')
        with parse_error(self) as stderr:
            parse_args([])
        self.assertRegexpMatches(stderr.getvalue(), 'Invalid JUJU_HOME value')
        if org:
            set_env('JUJU_HOME', org)

    def test_main(self):
        org = os.environ.get('JUJU_HOME')
        with temp_dir() as juju_home:
            set_env('JUJU_HOME', juju_home)
            temp_file = NamedTemporaryFile(delete=False)
            with patch('run_download_juju.NamedTemporaryFile', autospec=True,
                       return_value=temp_file) as ntf_mock:
                with patch('subprocess.check_output') as sco_mock:
                    main([])
            calls = [
                call(['workspace-run', temp_file.name,
                      'Administrator@win-slave.vapour.ws']),
                call(['workspace-run', temp_file.name,
                      'jenkins@osx-slave.vapour.ws'])]
        set_env('JUJU_HOME', org)
        self.assertEqual(sco_mock.call_args_list, calls)
        ntf_mock.assert_called_once_with()

    def test_main_expected_arguments(self):
        org = os.environ.get('JUJU_HOME')
        with temp_dir() as juju_home:
            set_env('JUJU_HOME', juju_home)
            temp_file = NamedTemporaryFile(delete=False)
            with patch('run_download_juju.get_revisions',
                       autospec=True) as gr_mock:
                with patch('run_download_juju.create_workspace_yaml',
                           autospec=True) as cwy_mock:
                    with patch('run_download_juju.NamedTemporaryFile',
                               autospec=True,
                               return_value=temp_file) as ntf_mock:
                        with patch('subprocess.check_output') as sco_mock:
                            main([])
            sco_calls = [
                call(['workspace-run', temp_file.name,
                      'Administrator@win-slave.vapour.ws']),
                call(['workspace-run', temp_file.name,
                      'jenkins@osx-slave.vapour.ws'])]
            cwy_calls = [
                call(juju_home,
                     ('C:\\\Users\\\Administrator\\\juju-ci-tools\\\download_'
                      'juju.py'),
                     temp_file, gr_mock.return_value),
                call(juju_home, '$HOME/juju-ci-tools/download_juju.py',
                     temp_file, gr_mock.return_value)]
        set_env('JUJU_HOME', org)
        self.assertEqual(sco_mock.call_args_list, sco_calls)
        ntf_mock.assert_called_once_with()
        gr_mock.assert_called_once_with(os.environ['HOME'])
        self.assertEqual(cwy_mock.call_args_list, cwy_calls)

    def expected(self, rev="''"):
        return ("command: [python, /script/path, cloud-city, -r, -v, {}]\n"
                "install:\n"
                "  cloud-city: [/my/home/ec2rc]\n".format(rev))


def del_env(key):
    try:
        org = os.environ[key]
        del os.environ[key]
    except KeyError:
        org = None
    return org


def set_env(key, value):
    if value is None:
        del os.environ[key]
    else:
        os.environ[key] = value
