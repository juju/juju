from contextlib import contextmanager
from mock import patch
import os
import shutil
from tempfile import mkdtemp
from unittest import TestCase


import winbuildtest
from winbuildtest import (
    build_agent,
    create_cloud_agent,
    GO_CMD,
    GOPATH,
)


@contextmanager
def temp_path(module, attr):
    dir_name = mkdtemp()
    old_path = getattr(module, attr)
    if os.path.isabs(old_path):
        rel_path = old_path[1:]
    else:
        rel_path = old_path
    new_path = os.path.join(dir_name, rel_path)
    os.makedirs(new_path)
    try:
        setattr(module, attr, new_path)
        yield new_path
    finally:
        setattr(module, attr, old_path)
        shutil.rmtree(dir_name)


class WinBuildTestTestCase(TestCase):

    def test_build_agent(self):
        with temp_path(winbuildtest, 'JUJUD_CMD_DIR'):
            with patch('winbuildtest.run') as run_mock:
                build_agent()
                args, kwargs = run_mock.call_args
                self.assertEqual((GO_CMD, 'build'), args)
                self.assertEqual('amd64', kwargs['env'].get('GOARCH'))
                self.assertEqual(GOPATH, kwargs['env'].get('GOPATH'))
