from mock import (
    Mock,
    patch,
    )
import os
from unittest import TestCase

from build_juju import (
    ARTIFACT_GLOBS,
    build_juju,
    get_crossbuild_script,
    get_juju_tarfile,
    main,
)
from utility import temp_dir


class JujuBuildTestCase(TestCase):

    def test_main_options(self):
        with patch('build_juju.build_juju', autospec=True) as mock:
            main(['-d', '-v', '-b', '1234', 'win-client', './foo'])
            s3cfg = os.path.join(os.environ.get('JUJU_HOME'), 'juju-qa.s3cfg')
            mock.assert_called_once_with(
                s3cfg, 'win-client', './foo', '1234',
                dry_run=True, juju_release_tools=None, verbose=True)

    def test_main_options_with_arch(self):
        with patch('build_juju.build_juju', autospec=True) as mock:
            main(['-d', '-v', '-b', '1234', '-a', 's390x',
                  'ubuntu-agent', './foo'])
            s3cfg = os.path.join(os.environ.get('JUJU_HOME'), 'juju-qa.s3cfg')
            mock.assert_called_once_with(
                s3cfg, 'ubuntu-agent', './foo', '1234',
                dry_run=True, juju_release_tools=None, verbose=True)

    def test_build_juju(self):
        with temp_dir() as base_dir:
            work_dir = os.path.join(base_dir, 'workspace')
            with patch('build_juju.setup_workspace', autospec=True) as sw_mock:
                with patch('build_juju.get_juju_tarfile',
                           return_value='juju-core_1.2.3.tar.gz',
                           autospec=True) as gt_mock:
                    with patch('build_juju.run_command') as rc_mock:
                        with patch('build_juju.add_artifacts', autospec=True
                                   ) as aa_mock:
                            build_juju(
                                's3cfg', 'win-client', work_dir, '1234',
                                dry_run=True, verbose=True)
        sw_mock.assert_called_once_with(
            work_dir, dry_run=True, verbose=True)
        gt_mock.assert_called_once_with('s3cfg', '1234', work_dir)
        crossbuild = get_crossbuild_script()
        rc_mock.assert_called_once_with(
            [crossbuild, '-b', '~/crossbuild', 'win-client',
             'juju-core_1.2.3.tar.gz'], dry_run=True, verbose=True)
        aa_mock.assert_called_once_with(
            work_dir, ARTIFACT_GLOBS, dry_run=True, verbose=True)

    def test_build_juju_with_goarch(self):
        with temp_dir() as base_dir:
            work_dir = os.path.join(base_dir, 'workspace')
            with patch('build_juju.setup_workspace', autospec=True):
                with patch('build_juju.get_juju_tarfile',
                           return_value='juju-core_1.2.3.tar.gz',
                           autospec=True):
                    with patch('build_juju.run_command') as rc_mock:
                        with patch('build_juju.add_artifacts', autospec=True):
                            build_juju(
                                's3cfg', 'ubuntu-agent', work_dir, '1234',
                                goarch='s390x', dry_run=True, verbose=True)
        crossbuild = get_crossbuild_script()
        rc_mock.assert_called_once_with(
            [crossbuild, '-b', '~/crossbuild', '--goarch', 's390x',
             'ubuntu-agent', 'juju-core_1.2.3.tar.gz'],
            dry_run=True, verbose=True)

    def test_get_crossbuild_script(self):
        self.assertEqual(
            '/foo/juju-release-tools/crossbuild.py',
            get_crossbuild_script('/foo/juju-release-tools'))
        parent_dir = os.path.realpath(
            os.path.join(__file__, '..', '..', '..'))
        self.assertEqual(
            os.path.join(parent_dir, 'juju-release-tools', 'crossbuild.py'),
            get_crossbuild_script())

    def test_get_juju_tarfile(self):
        s3cfg_path = './cloud-city/juju-qa.s3cfg'
        bucket = Mock()
        with patch('build_juju.s3ci.get_qa_data_bucket',
                   return_value=bucket, autospec=True) as gb_mock:
            with patch('build_juju.s3ci.fetch_files', autospec=True,
                       return_value=['./juju-core_1.2.3.tar.gz']) as ff_mock:
                tarfile = get_juju_tarfile(s3cfg_path, '1234', './')
        self.assertEqual('./juju-core_1.2.3.tar.gz', tarfile)
        gb_mock.assert_called_once_with('./cloud-city/juju-qa.s3cfg')
        ff_mock.assert_called_once_with(
            bucket, '1234', 'build-revision', 'juju-core_.*.tar.gz', './')
