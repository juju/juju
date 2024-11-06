// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

// These commands are deprecated, we don't want to document them.
var ignoreCommands = []string{"k8s-raw-get", "k8s-raw-set", "k8s-spec-get",
	"k8s-spec-set", "pod-spec-get", "pod-spec-set"}

// This script generates Markdown documentation for the hook tools.
// We want to use the same `juju documentation` command to generate docs for
// the hook tools, to ensure the hook tool docs are consistent with the command
// docs. However, there is no super-command for the hook tools like there is
// with juju commands. Hence, this script creates such a super-command, and
// runs the embedded 'documentation' command to generate the docs.
func main() {
	baseDocsDir := mustEnv("DOCS_DIR")
	hookToolDocsDir := filepath.Join(baseDocsDir, "hook-tools")

	// We need to transform the discourse-topic-ids.yaml into a format that can
	// be accepted by the juju documentation command.
	discourseIDs := translateDiscourseIDs()

	jujucSuperCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
	for _, name := range jujuc.CommandNames() {
		if slices.Contains(ignoreCommands, name) {
			continue
		}
		hookTool, err := jujuc.NewCommand(dummyHookContext{}, name)
		check(err)
		jujucSuperCmd.Register(hookTool)
	}

	jujucSuperCmd.SetFlags(&gnuflag.FlagSet{})
	err := jujucSuperCmd.Init([]string{"documentation", "--split",
		"--out", hookToolDocsDir,
		"--discourse-ids", discourseIDs,
	})
	check(err)
	err = jujucSuperCmd.Run(&cmd.Context{})
	check(err)

	// Delete the default help.md and documentation.md files that are generated
	// automatically. They are contrived here and don't have any meaning.
	err = os.Remove(filepath.Join(hookToolDocsDir, "documentation.md"))
	check(err)
	err = os.Remove(filepath.Join(hookToolDocsDir, "help.md"))
	check(err)
}

// Extracts the Discourse IDs relating to hook tools. Returns a filepath
// pointing to the filtered map of IDs.
func translateDiscourseIDs() string {
	allDiscourseIDs := mustEnv("TOPIC_IDS")
	file, err := os.Open(allDiscourseIDs)
	check(err)

	allIDs := map[string]int{}
	err = yaml.NewDecoder(file).Decode(&allIDs)
	check(err)

	// Filter out hook tool IDs
	hookToolIDs := map[string]int{}
	for fullname, id := range allIDs {
		if hookToolName, ok := strings.CutPrefix(fullname, "hook-tools/"); ok {
			hookToolIDs[hookToolName] = id
		}
	}

	newFile, err := os.CreateTemp("", "topic_ids")
	check(err)
	err = yaml.NewEncoder(newFile).Encode(hookToolIDs)
	check(err)
	check(newFile.Close())
	return newFile.Name()
}

// dummyHookContext implements hooks.Context, as expected by hooks.NewCommand.
// Copied from cmd/juju/commands/helptool.go
type dummyHookContext struct{ jujuc.Context }

func (dummyHookContext) AddMetrics(_, _ string, _ time.Time) error {
	return nil
}
func (dummyHookContext) UnitName() string {
	return ""
}
func (dummyHookContext) SetPodSpec(specYaml string) error {
	return nil
}
func (dummyHookContext) GetPodSpec() (string, error) {
	return "", nil
}
func (dummyHookContext) SetRawK8sSpec(specYaml string) error {
	return nil
}
func (dummyHookContext) GetRawK8sSpec() (string, error) {
	return "", nil
}
func (dummyHookContext) PublicAddress(ctx context.Context) (string, error) {
	return "", errors.NotFoundf("PublicAddress")
}
func (dummyHookContext) PrivateAddress() (string, error) {
	return "", errors.NotFoundf("PrivateAddress")
}
func (dummyHookContext) AvailabilityZone() (string, error) {
	return "", errors.NotFoundf("AvailabilityZone")
}
func (dummyHookContext) OpenPort(protocol string, port int) error {
	return nil
}
func (dummyHookContext) ClosePort(protocol string, port int) error {
	return nil
}
func (dummyHookContext) OpenedPorts() []network.PortRange {
	return nil
}
func (dummyHookContext) ConfigSettings(ctx context.Context) (charm.Settings, error) {
	return charm.NewConfig().DefaultSettings(), nil
}
func (dummyHookContext) HookRelation() (jujuc.ContextRelation, error) {
	return nil, errors.NotFoundf("HookRelation")
}
func (dummyHookContext) RemoteUnitName() (string, error) {
	return "", errors.NotFoundf("RemoteUnitName")
}
func (dummyHookContext) RemoteApplicationName() (string, error) {
	return "", errors.NotFoundf("RemoteApplicationName")
}
func (dummyHookContext) Relation(id int) (jujuc.ContextRelation, error) {
	return nil, errors.NotFoundf("Relation")
}
func (dummyHookContext) RelationIds(context.Context) ([]int, error) {
	return []int{}, errors.NotFoundf("RelationIds")
}
func (dummyHookContext) RequestReboot(_ context.Context, prio jujuc.RebootPriority) error {
	return nil
}
func (dummyHookContext) HookStorageInstance() (*storage.StorageInstance, error) {
	return nil, errors.NotFoundf("HookStorageInstance")
}
func (dummyHookContext) HookStorage(ctx context.Context) (jujuc.ContextStorageAttachment, error) {
	return nil, errors.NotFoundf("HookStorage")
}
func (dummyHookContext) StorageInstance(id string) (*storage.StorageInstance, error) {
	return nil, errors.NotFoundf("StorageInstance")
}
func (dummyHookContext) UnitStatus(ctx context.Context) (*jujuc.StatusInfo, error) {
	return &jujuc.StatusInfo{}, nil
}
func (dummyHookContext) SetStatus(jujuc.StatusInfo) error {
	return nil
}

// UTILITY FUNCTIONS

// check panics if the provided error is not nil.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

// Returns the value of the given environment variable, panicking if the var
// is not set.
func mustEnv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		panic(fmt.Sprintf("env var %q not set", key))
	}
	return val
}
