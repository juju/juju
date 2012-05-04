package upstart

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

var startedRE = regexp.MustCompile("^.* start/running, process (\\d+)\n$")

// Service provides visibility into and control over an upstart service.
type Service struct {
	Name    string
	InitDir string // defaults to "/etc/init"
}

func NewService(name string) *Service {
	return &Service{Name: name, InitDir: "/etc/init"}
}

// path returns the path to the service's configuration file.
func (s *Service) path() string {
	return filepath.Join(s.InitDir, s.Name+".conf")
}

// pid returns the Service's current pid, or -1 if it cannot be determined.
func (s *Service) pid() int {
	cmd := exec.Command("status", s.Name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return -1
	}
	match := startedRE.FindStringSubmatch(string(out))
	if match == nil {
		return -1
	}
	pid, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return pid
}

// Installed returns true if the Service appears to be installed.
func (s *Service) Installed() bool {
	_, err := os.Stat(s.path())
	return err == nil
}

// Running returns true if the Service appears to be running.
func (s *Service) Running() bool {
	return s.pid() != -1
}

// Stable returns true if the Service appears to be running stably, by
// checking that the reported pid does not change over the course of 5
// checks over 0.4 seconds.
func (s *Service) Stable() bool {
	pid := s.pid()
	if pid == -1 {
		return false
	}
	for i := 0; i < 4; i++ {
		<-time.After(100 * time.Millisecond)
		if s.pid() != pid {
			return false
		}
	}
	return true
}

// Start starts the service.
func (s *Service) Start() error {
	if s.Running() {
		return nil
	}
	return exec.Command("start", s.Name).Run()
}

// Stop stops the service.
func (s *Service) Stop() error {
	if !s.Running() {
		return nil
	}
	return exec.Command("stop", s.Name).Run()
}

// Remove removes the service.
func (s *Service) Remove() error {
	if !s.Installed() {
		return nil
	}
	if err := s.Stop(); err != nil {
		return err
	}
	return os.Remove(s.path())
}
