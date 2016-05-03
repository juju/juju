from mock import patch
import os
import shutil
import tarfile
from unittest import TestCase

from gotesttarfile import (
    go_test_package,
    main,
    parse_args,
    run,
    untar_gopath,
)
from utility import temp_dir


class FakePopen(object):

    def __init__(self, code):
        self._code = code

    def communicate(self):
        self.returncode = self._code
        return None, None


class gotesttarfileTestCase(TestCase):

    def test_run_success(self):
        env = {'a': 'b'}
        with patch('subprocess.Popen', autospec=True,
                   return_value=FakePopen(0)) as mock:
            returncode = run(['go', 'test', './...'], env=env)
        self.assertEqual(0, returncode)
        mock.assert_called_once_with(['go', 'test', './...'], env=env)

    def test_run_fail(self):
        env = {'a': 'b'}
        with patch('subprocess.Popen', autospec=True,
                   return_value=FakePopen(1)) as mock:
            returncode = run(['go', 'test', './...'], env=env)
        self.assertEqual(1, returncode)
        mock.assert_called_once_with(['go', 'test', './...'], env=env)

    def test_untar_gopath(self):
        with temp_dir() as base_dir:
            old_dir = os.path.join(base_dir, 'juju_1.2.3')
            for sub in ['bin', 'pkg', 'src']:
                os.makedirs(os.path.join(old_dir, sub))
            tarfile_path = os.path.join(base_dir, 'juju_1.2.3.tar.gz')
            with tarfile.open(name=tarfile_path, mode='w:gz') as tar:
                tar.add(old_dir, arcname='juju_1.2.3')
            shutil.rmtree(old_dir)
            gopath = os.path.join(base_dir, 'gogo')
            untar_gopath(tarfile_path, gopath, delete=True)
            self.assertTrue(os.path.isdir(gopath))
            self.assertItemsEqual(['bin', 'src', 'pkg'], os.listdir(gopath))
            self.assertFalse(os.path.isfile(tarfile_path))

    def test_go_test_package(self):
        with temp_dir() as gopath:
            package_path = os.path.join(
                gopath, 'src', 'github.com', 'juju', 'juju')
            os.makedirs(package_path)
            with patch('gotesttarfile.run', return_value=0,
                       autospec=True) as run_mock:
                devnull = open(os.devnull, 'w')
                with patch('sys.stdout', devnull):
                    returncode = go_test_package(
                        'github.com/juju/juju', 'go', gopath)
        self.assertEqual(0, returncode)
        self.assertEqual(run_mock.call_count, 2)
        args, kwargs = run_mock.call_args_list[0]
        self.assertEqual(
            (['go', 'test', '-timeout=1200s', './...'],), args)
        self.assertEqual('amd64', kwargs['env'].get('GOARCH'))
        self.assertEqual(gopath, kwargs['env'].get('GOPATH'))
        run_mock.assert_called_with(['sudo', 'killall', '-SIGABRT', 'mongod'])

    def test_go_test_package_win32(self):
        with temp_dir() as gopath:
            package_path = os.path.join(
                gopath, 'src', 'github.com', 'juju', 'juju')
            os.makedirs(package_path)
            with patch('gotesttarfile.run', return_value=0,
                       autospec=True) as run_mock:
                devnull = open(os.devnull, 'w')
                with patch('sys.stdout', devnull):
                    with patch('sys.platform', 'win32'):
                        tainted_path = r'C:\foo;C:\bar\OpenSSH;C:\baz'
                        with patch.dict(os.environ, {'PATH': tainted_path}):
                            returncode = go_test_package(
                                'github.com/juju/juju', 'go', gopath)
        self.assertEqual(0, returncode)
        args, kwargs = run_mock.call_args_list[0]
        self.assertEqual(run_mock.call_count, 2)
        self.assertEqual(
            (['powershell.exe', '-Command', 'go', 'test',
              '-timeout=1200s', './...'], ),
            args)
        self.assertEqual(r'C:\foo;C:\baz', kwargs['env'].get('Path'))
        self.assertEqual(None, kwargs['env'].get('PATH'))
        self.assertEqual(gopath, os.path.dirname(kwargs['env'].get('TMP')))
        self.assertIn("tmp-juju-", os.path.basename(kwargs['env'].get('TMP')))
        self.assertEqual(kwargs['env'].get('TEMP'), kwargs['env'].get('TMP'))
        run_mock.assert_called_with(
            ['taskkill.exe', '/F', '/FI', 'imagename eq mongod.exe'])

    def test_parse_args(self):
        args = parse_args(
            ['-v', '-g', 'go', '-p' 'github/foo', '-r', 'juju.tar.gz'])
        self.assertTrue(args.verbose)
        self.assertEqual('go', args.go)
        self.assertEqual('github/foo', args.package)
        self.assertTrue(args.remove_tarfile)
        self.assertEqual('juju.tar.gz', args.tarfile)

    def test_main(self):
        with patch('gotesttarfile.untar_gopath', autospec=True) as ug_mock:
            with patch('gotesttarfile.go_test_package',
                       autospec=True, return_value=0) as gt_mock:
                returncode = main(['/juju.tar.gz'])
        self.assertEqual(0, returncode)
        args, kwargs = ug_mock.call_args
        self.assertEqual('/juju.tar.gz', args[0])
        gopath = args[1]
        self.assertEqual('gogo', gopath.split(os.sep)[-1])
        self.assertFalse(kwargs['delete'])
        self.assertFalse(kwargs['verbose'])
        args, kwargs = gt_mock.call_args
        self.assertEqual(('github.com/juju/juju', 'go', gopath), args)
        self.assertFalse(kwargs['verbose'])
