// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshexec

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/exec"
)

func HandleSSHConn(k8s kubernetes.Interface, config *rest.Config, namespace string, pod string, container string,
	conn net.Conn, hostKey gossh.Signer, authorizedKeys []gossh.PublicKey) {
	sshd := ssh.Server{}
	sshd.HostSigners = []ssh.Signer{hostKey}
	sshd.PublicKeyHandler = func(ctx ssh.Context, untrustedKey ssh.PublicKey) bool {
		for _, authorizedKey := range authorizedKeys {
			if ssh.KeysEqual(authorizedKey, untrustedKey) {
				return true
			}
		}
		return false
	}
	sshd.ChannelHandlers = map[string]ssh.ChannelHandler{
		"session": ssh.DefaultSessionHandler,
	}
	sshd.Handle(func(sess ssh.Session) {
		pty, window, isTty := sess.Pty()

		command := sess.Command()
		if len(command) == 0 {
			command = []string{"sh"}
		}

		req := k8s.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(pod).
			Namespace(namespace).
			SubResource("exec").
			Param("container", container).
			VersionedParams(&corev1.PodExecOptions{
				Container: container,
				Command:   command,
				Stdin:     true,
				Stdout:    true,
				Stderr:    !isTty,
				TTY:       isTty,
			}, scheme.ParameterCodec)
		executor, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
		if err != nil {
			err = fmt.Errorf("cannot create spdy executor: %w", err)
			fmt.Fprintln(sess, err)
			sess.Exit(1)
			return
		}

		var tsq *termSize
		if isTty {
			tsq = &termSize{
				initial: &pty.Window,
				changes: window,
			}
		}

		streamOpts := remotecommand.StreamOptions{
			Stdin:             sess,
			Stdout:            sess,
			Tty:               isTty,
			TerminalSizeQueue: tsq,
		}
		if !isTty {
			streamOpts.Stderr = sess.Stderr()
		}

		err = executor.StreamWithContext(sess.Context(), streamOpts)
		var statusError *exec.CodeExitError
		if errors.As(err, &statusError) {
			fmt.Fprintln(sess, statusError)
			sess.Exit(statusError.Code)
			return
		} else if err != nil {
			err = fmt.Errorf("cannot open stream: %w", err)
			fmt.Fprintln(sess, err)
		}
		sess.Exit(1)
	})
	sshd.SubsystemHandlers = map[string]ssh.SubsystemHandler{
		"sftp": func(sess ssh.Session) {
			h := &sftpHandler{
				k8s:       k8s,
				config:    config,
				namespace: namespace,
				pod:       pod,
				container: container,
			}
			sftpServer := sftp.NewRequestServer(sess, sftp.Handlers{
				FileGet:  h,
				FilePut:  h,
				FileCmd:  h,
				FileList: h,
			})
			err := sftpServer.Serve()
			if err != nil && !errors.Is(err, io.EOF) {
				err = fmt.Errorf("cannot serve sftp sessions: %w", err)
				fmt.Fprintln(sess, err)
				sess.Exit(1)
				return
			}
			sess.Exit(0)
		},
	}
	sshd.HandleConn(conn)
}

type termSize struct {
	initial *ssh.Window
	changes <-chan ssh.Window
}

func (t *termSize) Next() *remotecommand.TerminalSize {
	if t.initial != nil {
		r := &remotecommand.TerminalSize{
			Width:  uint16(t.initial.Width),
			Height: uint16(t.initial.Height),
		}
		t.initial = nil
		return r
	}

	w, ok := <-t.changes
	if !ok {
		return nil
	}

	return &remotecommand.TerminalSize{
		Width:  uint16(w.Width),
		Height: uint16(w.Height),
	}
}

type sftpHandler struct {
	k8s       kubernetes.Interface
	config    *rest.Config
	namespace string
	pod       string
	container string
}

func (h *sftpHandler) Fileread(req *sftp.Request) (io.ReaderAt, error) {
	tempFile, err := os.CreateTemp("", "sftp-temp-*")
	if err != nil {
		return nil, fmt.Errorf("cannot create temp file: %w", err)
	}
	go func() {
		<-req.Context().Done()
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()
	err = h.exec(req.Context(), nil, tempFile, []string{"cat", req.Filepath})
	if err != nil {
		return nil, fmt.Errorf("cannot exec cat: %w", err)
	}
	_, err = tempFile.Seek(0, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot seek temp file back: %w", err)
	}
	return tempFile, nil
}

func (h *sftpHandler) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	flags := req.Pflags()
	if flags.Trunc && flags.Creat {
		err := h.exec(req.Context(), nil, nil, []string{"truncate", req.Filepath})
		if err != nil {
			return nil, fmt.Errorf("cannot exec truncate: %w", err)
		}
	} else if flags.Trunc && !flags.Creat {
		err := h.exec(req.Context(), nil, nil, []string{"truncate", "-c", req.Filepath})
		if err != nil {
			return nil, fmt.Errorf("cannot excec truncate without create: %w", err)
		}
	} else if !flags.Trunc && flags.Creat {
		err := h.exec(req.Context(), nil, nil, []string{"touch", req.Filepath})
		if err != nil {
			return nil, fmt.Errorf("cannot exec touch: %w", err)
		}
	}
	if flags.Append {
		f := writerAtFunc(func(p []byte, off int64) (int, error) {
			err := h.exec(req.Context(), bytes.NewReader(p), nil, []string{"dd", "bs=1", "conv=nocreat", "oflag=append", "of=" + req.Filepath})
			if err != nil {
				return 0, fmt.Errorf("cannot exec dd with append: %w", err)
			}
			return len(p), nil
		})
		return f, nil
	}
	f := writerAtFunc(func(p []byte, off int64) (int, error) {
		err := h.exec(req.Context(), bytes.NewReader(p), nil, []string{"dd", "bs=1", "seek=" + strconv.FormatInt(off, 10), "conv=nocreat", "of=" + req.Filepath})
		if err != nil {
			return 0, fmt.Errorf("cannot exec dd: %w", err)
		}
		return len(p), nil
	})
	return f, nil
}

func (h *sftpHandler) Filecmd(req *sftp.Request) error {
	switch req.Method {
	case "Setstat":
		if req.AttrFlags().Size {
			err := h.exec(req.Context(), nil, nil, []string{"truncate", "-c", "-s", strconv.FormatUint(req.Attributes().Size, 10), req.Filepath})
			if err != nil {
				return fmt.Errorf("cannot exec truncate for Setstat: %w", err)
			}
		}
		if req.AttrFlags().Acmodtime {
			err := h.exec(req.Context(), nil, nil, []string{"touch", "-c", "-m", "-t", time.Unix(int64(req.Attributes().Mtime), 0).Format("200601021504.05"), req.Filepath})
			if err != nil {
				return fmt.Errorf("cannot exec touch for Setstat: %w", err)
			}
		}
		if req.AttrFlags().Permissions {
			err := h.exec(req.Context(), nil, nil, []string{"chmod", strconv.FormatUint(uint64(req.Attributes().Mode), 8), req.Filepath})
			if err != nil {
				return fmt.Errorf("cannot exec chmod for Setstat: %w", err)
			}
		}
		if req.AttrFlags().UidGid {
			err := h.exec(req.Context(), nil, nil, []string{"chown", fmt.Sprintf("%d:%d", req.Attributes().UID, req.Attributes().GID), req.Filepath})
			if err != nil {
				return fmt.Errorf("cannot exec chown for Setstat: %w", err)
			}
		}
		return nil
	case "Mkdir":
		err := h.exec(req.Context(), nil, nil, []string{"mkdir", req.Filepath})
		if err != nil {
			return fmt.Errorf("cannot exec mkdir: %w", err)
		}
		return nil
	}
	return fmt.Errorf("unsupported Filecmd: %s", req.Method)
}

func (h *sftpHandler) Filelist(req *sftp.Request) (sftp.ListerAt, error) {
	entInfo, err := h.stat(req.Context(), req.Filepath)
	if err != nil {
		return nil, fmt.Errorf("cannot stat: %w", err)
	}
	if !entInfo.IsDir() {
		f := ListAtFunc(func(fi []os.FileInfo, i int64) (int, error) {
			fi[0] = entInfo
			return 1, io.EOF
		})
		return f, nil
	}

	buffer := &bytes.Buffer{}
	err = h.exec(req.Context(), nil, buffer, []string{"ls", "-1", req.Filepath})
	if err != nil {
		return nil, fmt.Errorf("cannot exec ls: %w", err)
	}
	reader := bufio.NewReader(buffer)
	var files []string
	for {
		l, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		fileName := strings.TrimSuffix(l, "\n")
		files = append(files, fileName)
	}

	f := ListAtFunc(func(fi []os.FileInfo, i int64) (int, error) {
		if i >= int64(len(files)) {
			return 0, io.EOF
		}
		filesLeft := files[i:]
		for k, fileName := range filesLeft {
			if k >= len(fi) {
				return k, io.EOF
			}
			filePath := path.Join(req.Filepath, fileName)
			fi[k], err = h.stat(req.Context(), filePath)
			if err != nil {
				return 0, fmt.Errorf("cannot stat: %w", err)
			}
		}
		return len(filesLeft), io.EOF
	})
	return f, nil
}

func (h *sftpHandler) stat(ctx context.Context, p string) (os.FileInfo, error) {
	statYaml := &bytes.Buffer{}
	err := h.exec(ctx, nil, statYaml, []string{"stat", "-c", "name: %n\nmode: %f\nsize: %s\nmod: %Y\ntype: %F\n", p})
	if err != nil {
		return nil, fmt.Errorf("cannot stat: %w", err)
	}
	var info struct {
		Name    string `yaml:"name"`
		Size    int64  `yaml:"size"`
		Mode    string `yaml:"mode"`
		Mod     int64  `yaml:"mod"`
		EntType string `yaml:"type"`
	}
	err = yaml.Unmarshal(statYaml.Bytes(), &info)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal stat yaml: %w", err)
	}
	fileInfo := &fileInfo{
		name:    path.Base(info.Name),
		size:    info.Size,
		modTime: time.Unix(info.Mod, 0),
		isDir:   info.EntType == "directory",
	}
	mode, err := strconv.ParseUint(info.Mode, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("cannot parse file mode for stat: %w", err)
	}
	fileInfo.mode = fileModeFromUnixMode(uint32(mode))
	return fileInfo, nil
}

func (h *sftpHandler) exec(ctx context.Context, stdin io.Reader, stdout io.Writer,
	command []string) error {
	req := h.k8s.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(h.pod).
		Namespace(h.namespace).
		SubResource("exec").
		Param("container", h.container).
		VersionedParams(&corev1.PodExecOptions{
			Container: h.container,
			Command:   command,
			Stdin:     stdin != nil,
			Stdout:    stdout != nil,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(h.config, "POST", req.URL())
	if err != nil {
		return err
	}

	stderr := &bytes.Buffer{}
	streamOpts := remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	err = executor.StreamWithContext(ctx, streamOpts)
	var statusError *exec.CodeExitError
	if errors.As(err, &statusError) {
		return fmt.Errorf("exec failed: %w\n%s", err, stderr.String())
	} else if err != nil {
		return err
	}

	return nil
}

type ListAtFunc func([]os.FileInfo, int64) (int, error)

func (f ListAtFunc) ListAt(fi []os.FileInfo, off int64) (int, error) {
	return f(fi, off)
}

type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (f *fileInfo) Name() string {
	return f.name
}

func (f *fileInfo) Size() int64 {
	return f.size
}

func (f *fileInfo) Mode() os.FileMode {
	return f.mode
}

func (f *fileInfo) ModTime() time.Time {
	return f.modTime
}

func (f *fileInfo) IsDir() bool {
	return f.isDir
}

func (f *fileInfo) Sys() any {
	return nil
}

// Copied mostly from go src
func fileModeFromUnixMode(mode uint32) os.FileMode {
	fileMode := os.FileMode(mode & 0777)
	switch mode & syscall.S_IFMT {
	case syscall.S_IFBLK:
		fileMode |= os.ModeDevice
	case syscall.S_IFCHR:
		fileMode |= os.ModeDevice | os.ModeCharDevice
	case syscall.S_IFDIR:
		fileMode |= os.ModeDir
	case syscall.S_IFIFO:
		fileMode |= os.ModeNamedPipe
	case syscall.S_IFLNK:
		fileMode |= os.ModeSymlink
	case syscall.S_IFREG:
		// nothing to do
	case syscall.S_IFSOCK:
		fileMode |= os.ModeSocket
	}
	if mode&syscall.S_ISGID != 0 {
		fileMode |= os.ModeSetgid
	}
	if mode&syscall.S_ISUID != 0 {
		fileMode |= os.ModeSetuid
	}
	if mode&syscall.S_ISVTX != 0 {
		fileMode |= os.ModeSticky
	}
	return fileMode
}

type writerAtFunc func(p []byte, off int64) (n int, err error)

func (w writerAtFunc) WriteAt(p []byte, off int64) (n int, err error) {
	return w(p, off)
}
