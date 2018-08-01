#!/usr/bin/env python

import os
import subprocess
import sys


KNOWN_LIVE_SUITES = [
    'client',
    'glance',
    'identity',
    'nova',
    'neutron',
    'swift',
]


def ensure_tarmac_log_dir():
    """Hack-around tarmac not creating its own log directory."""
    try:
        os.makedirs(os.path.expanduser("~/logs/"))
    except OSError:
        # Could be already exists, or cannot create, either way, just continue
        pass


def create_tarmac_repository():
    """Try to ensure a shared repository for the code."""
    try:
        from bzrlib import (
            branch,
            controldir,
            errors,
            transport,
            repository,
            reconfigure,
            )
    except ImportError:
        sys.stderr.write('Could not import bzrlib to ensure a repository\n')
        return
    try:
        b, _ = branch.Branch.open_containing('.')
    except:
        sys.stderr.write('Could not open local branch\n')
        return
    # By the time we get here, we've already branched everything from
    # launchpad. So if we aren't in a shared repository, we create one, and
    # fetch all the data into it, so it doesn't have to be fetched again.
    if b.repository.is_shared():
        return
    pwd = os.getcwd()
    expected_dir = 'src/github.com/'
    offset = pwd.rfind(expected_dir)
    if offset == -1:
        sys.stderr.write('Could not find %r to create a shared repo\n')
        return
    path = pwd[:offset+len(expected_dir)]
    try:
        repository.Repository.open(path)
    except (errors.NoRepositoryPresent, errors.NotBranchError):
        pass # Good, the repo didn't exist
    else:
        # We must have already created the repo.
        return
    repo_fmt = controldir.format_registry.make_bzrdir('default')
    trans = transport.get_transport(path)
    info = repo_fmt.initialize_on_transport_ex(trans, create_prefix=False,
        make_working_trees=True, shared_repo=True, force_new_repo=True,
        use_existing_dir=True,
        repo_format_name=repo_fmt.repository_format.get_format_string())
    repo = info[0]
    sys.stderr.write('Reconfiguring to use a shared repository\n')
    reconfiguration = reconfigure.Reconfigure.to_use_shared(b.bzrdir)
    try:
        reconfiguration.apply(False)
    except errors.NoRepositoryPresent:
        sys.stderr.write('tarmac did a lightweight checkout,'
                         ' not fetching into the repo.\n')


def ensure_juju_core_dependencies():
    """Ensure that juju-core and all dependencies have been installed."""
    # Note: This potentially overwrites goose while it is updating the world.
    # However, if we are targetting the trunk branch of goose, that should have
    # already been updated to the latest version by tarmac.
    # I don't quite see a way to reconcile that we want the latest juju-core
    # and all of the other dependencies, but we don't want to touch goose
    # itself. One option would be to have a split GOPATH. One installs the
    # latest juju-core and everything else. The other is where the
    # goose-under-test resides. So we don't add the goose-under-test to GOPATH,
    # call "go get", then add it to the GOPATH for the rest of the testing.
    cmd = ['go', 'get', '-u', '-x', 'github.com/juju/...']
    sys.stderr.write('Running: %s\n' % (' '.join(cmd),))
    retcode = subprocess.call(cmd)
    if retcode != 0:
        sys.stderr.write('WARN: Failed to update github.com/juju\n')


def tarmac_setup(opts):
    """Do all the bits of setup that need to happen for the tarmac bot."""
    ensure_tarmac_log_dir()
    create_tarmac_repository()


def setup_gopath():
    pwd = os.getcwd()
    if sys.platform == 'win32':
        pwd = pwd.replace('\\', '/')
    offset = pwd.rfind('src/gopkg.in/goose.v2')
    if offset == -1:
        sys.stderr.write('Could not find "src/gopkg.in/goose.v2" in cwd: %s\n'
                         % (pwd,))
        sys.stderr.write('Unable to automatically set GOPATH\n')
        return
    add_gopath = pwd[:offset].rstrip('/')
    gopath = os.environ.get("GOPATH")
    if gopath:
        if add_gopath in gopath:
            return
        # Put this path first, so we know we are running these tests
        gopath = add_gopath + os.pathsep + gopath
    else:
        gopath = add_gopath
    sys.stderr.write('Setting GOPATH to: %s\n' % (gopath,))
    os.environ['GOPATH'] = gopath


def run_cmd(cmd):
    cmd_str = ' '.join(cmd)
    sys.stderr.write('Running: %s\n' % (cmd_str,))
    retcode = subprocess.call(cmd)
    if retcode != 0:
        sys.stderr.write('FAIL: failed running: %s\n' % (cmd_str,))
    return retcode


def run_go_fmt(opts):
    return run_cmd(['go', 'fmt', './...'])


def run_go_build(opts):
    return run_cmd(['go', 'build', './...'])


def run_go_test(opts):
    # Note: I wish we could run this with '-check.v'
    return run_cmd(['go', 'test', './...'])


def run_juju_core_tests(opts):
    """Run the juju-core test suite"""
    orig_wd = os.getcwd()
    try:
        sys.stderr.write('Switching to juju-core\n')
        os.chdir('../juju-core')
        retval = run_cmd(['go', 'build', './...'])
        if retval != 0:
            return retval
        return run_cmd(['go', 'test', './...'])
    finally:
        os.chdir(orig_wd)


def run_live_tests(opts):
    """Run all of the live tests."""
    orig_wd = os.getcwd()
    final_retcode = 0
    for d in KNOWN_LIVE_SUITES:
        try:
            cmd = ['go', 'test', '-live', '-check.v']
            sys.stderr.write('Running: %s in %s\n' % (' '.join(cmd), d))
            os.chdir(d)
            retcode = subprocess.call(cmd)
            if retcode != 0:
                sys.stderr.write('FAIL: Running live tests in %s\n' % (d,))
                final_retcode = retcode
        finally:
            os.chdir(orig_wd)
    return final_retcode


def main(args):
    import argparse
    p = argparse.ArgumentParser(description='Run the goose test suite')
    p.add_argument('--verbose', action='store_true', help='Be chatty')
    p.add_argument('--version', action='version', version='%(prog)s 0.1')
    p.add_argument('--tarmac', action='store_true',
        help="Pass this if the script is running as the tarmac bot."
             " This is used for stuff like ensuring repositories and"
             " logging directories are initialized.")
    p.add_argument('--juju-core', action='store_true',
        help="Run the juju-core trunk tests as well as the goose tests.")
    p.add_argument('--live', action='store_true',
        help="Run tests against a live service.")

    opts = p.parse_args(args)
    setup_gopath()
    if opts.tarmac:
        tarmac_setup(opts)
    to_run = [run_go_fmt, run_go_build, run_go_test]
    if opts.juju_core:
        ensure_juju_core_dependencies()
        to_run.append(run_juju_core_tests)
    if opts.live:
        to_run.append(run_live_tests)
    for func in to_run:
        retcode = func(opts)
        if retcode != 0:
            return retcode


if __name__ == '__main__':
    import sys
    sys.exit(main(sys.argv[1:]))

