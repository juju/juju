import json
import os
import shutil
import subprocess

from jujupy import ensure_dir
from utility import(
    find_candidates,
    s3_cmd,
    run_command,
)


def cmd(cmd_str, verbose=False):
    """ Run shell command with retries when a connection fails. """
    cmd_str = cmd_str.split() if isinstance(cmd_str, str) else cmd_str
    for i in range(3):
        try:
            return run_command(cmd_str, verbose=verbose)
        except subprocess.CalledProcessError as e:
            if e.returncode not in (1, 255):
                raise
    else:
        raise


def prepare_old_juju(root_dir, osx_host_info, win_host_info):
    old_juju_dir = os.path.join(root_dir, 'old-juju')
    ensure_dir(old_juju_dir)
    # Sync old-juju to local directory.
    s3_cmd(['sync', 's3://juju-qa-data/client-archive/win', old_juju_dir])
    s3_cmd(['sync', 's3://juju-qa-data/client-archive/osx', old_juju_dir])

    # Copy old-juju from local directory to OS X machine using rsync.
    osx_dir = os.path.join(old_juju_dir, 'osx')
    cmd('rsync -avz --delete {} {}:old-juju/'.format(osx_dir, osx_host_info),
        verbose=True)

    # Create old-juju directory on win machine.
    cmd('ssh {} mkdir -p old-juju'.format(win_host_info), verbose=True)
    cmd('ssh {} mkdir -p old-juju/win'.format(win_host_info), verbose=True)

    # scp can copy a directory using -r, but connection is not stable, Copy
    # one file at a time.
    win_dir = os.path.join(old_juju_dir, 'win')
    for juju in os.listdir(win_dir):
        juju_path = os.path.join(win_dir, juju)
        cmd('scp {} {}:old-juju/win/'.format(juju_path, win_host_info), verbose=True)


def get_revision_numbers():
    candidates = find_candidates(os.environ['HOME'])
    rev = []
    for candidate in candidates:
        buildvar = os.path.join(candidate, 'buildvars.json')
        with open(buildvar) as fp:
            rev.append(json.load(fp)['revision_build'])
    return rev


def prepare_candidates(root_dir, osx_host_info, osx_win_info):
    candidate_dir = os.path.join(root_dir, 'candidate')
    shutil.rmtree(candidate_dir, ignore_errors=True)
    win = os.path.join(candidate_dir, 'win')
    osx = os.path.join(candidate_dir, 'osx')
    ensure_dir(candidate_dir)
    ensure_dir(win)
    ensure_dir(osx)
    revs = get_revision_numbers()
    # Get candidate Juju from S3.
    for rev in revs:
        for os_str, build_dir, file_find in [[win, 'build-win-client', '*.exe'], [osx, 'build-osx-client', '*.tar.gz']]:
            rev_dir = os.path.join(os_str, 'rev-{}'.format(rev))
            ensure_dir(rev_dir)
            s3_cmd([
                'sync',
                's3://juju-qa-data/juju-ci/products/version-{}/{}'.format(
                    rev, build_dir),
                rev_dir, "--exclude", "*", "--include", file_find
            ])
    win_juju = run_command('find {} -name juju*.exe'.format(win).split(),
        verbose=True).split()
    osx_juju = run_command('find {} -name juju*.tar.gz'.format(osx).split(),
        verbose=True).split()

    # Copy candidates from local to remote.
    cmd('ssh {} mkdir -p candidate/osx'.format(osx_host_info), verbose=True)
    for juju in osx_juju:
        cmd('scp {} {}:candidate/osx/'.format(juju, osx_host_info),
            verbose=True)
    cmd('ssh {} mkdir -p candidate/win'.format(win_host_info), verbose=True)
    for juju in win_juju:
        cmd('scp {} {}:candidate/win/'.format(juju, win_host_info),
            verbose=True)



if __name__ == '__main__':
    osx_host_info = 'jenkins@osx-slave.vapour.ws'
    win_host_info = 'Administrator@52.6.148.61'
    root_dir = '/tmp/prepare-cs'
    ensure_dir(root_dir)
    prepare_old_juju(root_dir, osx_host_info, win_host_info)
    prepare_candidates(root_dir, osx_host_info, win_host_info)

