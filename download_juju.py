from argparse import ArgumentParser
import errno
import logging
import os
import socket
import urlparse

from boto.s3.connection import S3Connection

from utility import configure_logging


def retry(retries=3):
    """Retries a function if it raised a socket reset exception. """
    def decorator(func):
        def decorated(*args, **kwargs):
            for x in range(retries):
                if x != 0:
                    logging.info('Retrying due to a socket failure.')
                try:
                    return func(*args, **kwargs)
                except socket.error as e:
                    if e.errno not in errno.ECONNRESET:
                        raise
            else:
                raise
        return decorated
    return decorator


@retry()
def s3_download_files(s3_path, access_key, secret_key, dst_dir=None,
                      file_ext=None, overwrite=False):
    """ Download all files from S3 path.

    If dst_dir is set, it writes the S3 file to the directory. If the file
    already exists, it is overwritten if overwrite is set True.
    If file_ext is specified, it only downloads files that match the extension.
    """
    logging.info('Connecting to {}'.format(s3_path))
    conn = S3Connection(access_key, secret_key)
    uri = urlparse.urlparse(s3_path)
    bucket = conn.get_bucket(uri.netloc)
    remote_files = []
    local_files = []
    if dst_dir:
        mkdir_p(dst_dir)
        local_files = os.listdir(dst_dir)
    for key in bucket.list(uri.path.strip('/')):
        filename = os.path.basename(key.name)
        if not filename:
            continue
        if file_ext and not filename.endswith(file_ext):
            continue
        remote_files.append(key.name)
        if dst_dir:
            dst_path = os.path.join(dst_dir, filename)
            logging.info('Copying file: {} -> {}'.format(key.name, dst_path))
            if overwrite or filename not in local_files:
                key.get_contents_to_filename(dst_path)
    return remote_files


def get_os_str(platform):
    if platform == 'win32':
        return 'win'
    elif platform == 'darwin':
        return 'osx'
    return None


def download_released_juju(platform, args):
    logging.info('Getting released Juju.')
    os_str = get_os_str(platform)
    dst_dir = os.path.join(os.environ['HOME'], 'old-juju', os_str)
    s3_path = "s3://juju-qa-data/client-archive/{}/".format(os_str)
    s3_download_files(s3_path, access_key=args.access_key,
                      secret_key=args.secret_key, dst_dir=dst_dir)


def download_candidates(platform, args):
    logging.info('Getting candidate Juju.')
    os_str = get_os_str(platform)
    dst_dir = os.path.join(os.environ['HOME'], 'candidate', os_str)
    file_ext = '.exe' if os_str == 'win' else '.tar.gz'
    for rev in args.revision:
        s3_path = ("s3://juju-qa-data/juju-ci/products/version-{}/build-{}-"
                   "client/".format(rev, os_str))
        builds = s3_download_files(
            s3_path, access_key=args.access_key, secret_key=args.secret_key,
            file_ext=file_ext)
        if not builds:
            raise Exception('Build revision not found: ', rev)
        build = select_build(builds)
        s3_download_files(
            build, access_key=args.access_key, secret_key=args.secret_key,
            file_ext=file_ext, dst_dir=dst_dir, overwrite=True)


def select_build(builds):
    """ Select greater build number. """
    #  builds has the following format:
    # ['juju-ci/products/version-3000/build-osx-client/build-838/client
    # /juju-1.25-alpha1-osx.tar.gz'],['juju-ci/products/version-3000/
    # build-osx-client/build-839/client/juju-1.25-alpha1-osx.tar.gz']
    build = sorted(
        builds, key=lambda x: int(x.split('/')[4].split('-')[1]))[-1]
    build = build.split('/')[:5]
    return 's3://juju-qa-data/{}'.format('/'.join(build))


def parse_args(args=None):
    parser = ArgumentParser("Download released and candidate Juju.")
    parser.add_argument('-a', '--access-key', help="AWS access key")
    parser.add_argument('-s', '--secret-key', help="AWS secret key")
    parser.add_argument('-r', '--revision', nargs='+',
                        help="List of candidate revision numbers")
    parser.add_argument('-v', '--verbose', action='count', default=0,
                        help="Increase verbosity of console output.")
    parsed_args = parser.parse_args(args)
    return parsed_args


def mkdir_p(path):
    try:
        os.makedirs(path)
    except OSError as e:
        if e.errno == errno.EEXIST and os.path.isdir(path):
            pass
        else:
            raise

if __name__ == '__main__':
    args = parse_args()
    configure_logging(max(logging.WARNING - 10 * args.verbose, logging.DEBUG))
    download_released_juju('win32', args)
    download_released_juju('darwin', args)
    if args.revision:
        download_candidates('darwin', args)
        download_candidates('win32', args)
