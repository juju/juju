from mock import patch
import os
from unittest import TestCase

from jujuci import (
    Artifact,
    Credentials,
    )
from build_juju import (
    ARTIFACT_GLOBS,
    build_juju,
    get_crossbuild_script,
    main,
)
from utility import temp_dir


class JujuBuildTestCase(TestCase):

    def test_main_options(self):
        with patch('build_juju.build_juju', autospec=True) as mock:
            main(['-d', '-v', '-b', '1234', 'win-client', './foo',
                  '--user', 'jrandom', '--password', 'password1'])
            args, kwargs = mock.call_args
            self.assertEqual(
                (Credentials('jrandom', 'password1'), 'win-client', './foo',
                 '1234'), args)
            self.assertTrue(kwargs['dry_run'])
            self.assertTrue(kwargs['verbose'])

    def test_build_juju(self):
        credentials = Credentials('jrandom', 'password1')
        with temp_dir() as base_dir:
            work_dir = os.path.join(base_dir, 'workspace')
            with patch('build_juju.setup_workspace', autospec=True) as sw_mock:
                artifacts = [
                    Artifact('juju-core_1.2.3.tar.gz', 'http:...')]
                with patch('build_juju.get_artifacts',
                           return_value=artifacts, autospec=True) as ga_mock:
                    with patch('build_juju.run_command') as rc_mock:
                        with patch('build_juju.add_artifacts', autospec=True
                                   ) as aa_mock:
                            build_juju(
                                credentials, 'win-client', work_dir,
                                'lastSucessful', dry_run=True, verbose=True)
        self.assertEqual((work_dir, ), sw_mock.call_args[0])
        self.assertEqual(
            {'dry_run': True, 'verbose': True}, sw_mock.call_args[1])
        self.assertEqual(
            (credentials, 'build-revision', 'lastSucessful',
             'juju-core_*.tar.gz', work_dir,),
            ga_mock.call_args[0])
        self.assertEqual(
            {'archive': False, 'dry_run': True, 'verbose': True},
            ga_mock.call_args[1])
        crossbuild = get_crossbuild_script()
        self.assertEqual(
            ([crossbuild, 'win-client', '-b', '~/crossbuild',
              'juju-core_1.2.3.tar.gz'], ),
            rc_mock.call_args[0])
        self.assertEqual(
            {'dry_run': True, 'verbose': True}, rc_mock.call_args[1])
        self.assertEqual((work_dir, ARTIFACT_GLOBS), aa_mock.call_args[0])
        self.assertEqual(
            {'dry_run': True, 'verbose': True}, aa_mock.call_args[1])

    def test_get_crossbuild_script(self):
        self.assertEqual(
            '/foo/juju-release-tools/crossbuild.py',
            get_crossbuild_script('/foo/juju-release-tools'))
        parent_dir = os.path.realpath(
            os.path.join(__file__, '..', '..', '..'))
        self.assertEqual(
            os.path.join(parent_dir, 'juju-release-tools', 'crossbuild.py'),
            get_crossbuild_script())
