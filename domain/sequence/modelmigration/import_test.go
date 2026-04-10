// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v12"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/machine"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/domain/sequence"
	"github.com/juju/juju/internal/testhelpers"
)

type importSuite struct {
	testhelpers.IsolationSuite

	importService *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportSequences(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Juju 3.x stores "next value to return", Juju 4.x stores "last value returned".
	// Input values are decremented by 1 during import.
	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		"foo": 0, // input 1 - 1
		"bar": 1, // input 2 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence("foo", 1)
	model.SetSequence("bar", 2)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesLegacyOperation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		operation.OperationSequenceNamespace.String(): 0, // input 1 - 1
		"bar": 1, // input 2 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence(legacyOperationSequenceName, 1)
	model.SetSequence("bar", 2)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesLegacyStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		storageSequenceNamespace: 41, // input 42 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence(legacyStorageSequenceName, 42)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesLegacyApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// "application-myapp" -> "application_myapp"
	expectedNamespace := sequence.MakePrefixNamespace(application.ApplicationSequenceNamespace, "myapp").String()

	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		expectedNamespace: 9, // input 10 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence("application-myapp", 10)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesLegacyApplicationWithHyphen(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// "application-my-cool-app" -> "application_my-cool-app"
	expectedNamespace := sequence.MakePrefixNamespace(application.ApplicationSequenceNamespace, "my-cool-app").String()

	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		expectedNamespace: 4, // input 5 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence("application-my-cool-app", 5)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesLegacyContainerLXD(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// "machine0lxdContainer" -> "machine_container_0"
	expectedNamespace := sequence.MakePrefixNamespace(machine.ContainerSequenceNamespace, "0").String()

	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		expectedNamespace: 2, // input 3 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence("machine0lxdContainer", 3)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesLegacyContainerKVM(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// "machine1kvmContainer" -> "machine_container_1"
	expectedNamespace := sequence.MakePrefixNamespace(machine.ContainerSequenceNamespace, "1").String()

	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		expectedNamespace: 6, // input 7 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence("machine1kvmContainer", 7)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesLegacyNestedContainer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// "machine1/lxd/0kvmContainer" -> "machine_container_1/lxd/0"
	expectedNamespace := sequence.MakePrefixNamespace(machine.ContainerSequenceNamespace, "1/lxd/0").String()

	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		expectedNamespace: 1, // input 2 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence("machine1/lxd/0kvmContainer", 2)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesMultipleLegacyConversions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appNamespace := sequence.MakePrefixNamespace(application.ApplicationSequenceNamespace, "ubuntu").String()
	containerNamespace := sequence.MakePrefixNamespace(machine.ContainerSequenceNamespace, "0").String()

	// All values are decremented by 1 during import (Juju 3 -> Juju 4 semantics).
	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		operation.OperationSequenceNamespace.String(): 99, // input 100 - 1
		storageSequenceNamespace:                      49, // input 50 - 1
		appNamespace:                                  24, // input 25 - 1
		containerNamespace:                            9,  // input 10 - 1
		"machine":                                     4,  // input 5 - 1
		"relation":                                    2,  // input 3 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence("task", 100)
	model.SetSequence("stores", 50)
	model.SetSequence("application-ubuntu", 25)
	model.SetSequence("machine0lxdContainer", 10)
	model.SetSequence("machine", 5)
	model.SetSequence("relation", 3)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesSkipsZeroValues(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Sequences with value 0 or negative should be skipped as they were never used.
	// Only "bar" with value 2 should be imported (as 1 after decrement).
	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		"bar": 1, // input 2 - 1
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence("foo", 0)  // Should be skipped
	model.SetSequence("bar", 2)  // Should be imported as 1
	model.SetSequence("baz", -1) // Should be skipped (negative)

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestConvertLegacySequenceNameUnchanged(c *tc.C) {
	// Test that sequences that don't match any legacy pattern are unchanged.
	tests := []string{
		"machine",
		"relation",
		"filesystem",
		"volume",
		"subnet",
		"block",
		"branch",
		"controller",
		"modelmigration",
		"random-sequence",
	}

	for _, name := range tests {
		c.Check(convertLegacySequenceName(name), tc.Equals, name,
			tc.Commentf("sequence name %q should remain unchanged", name))
	}
}

func (s *importSuite) TestConvertLegacySequenceNameOperation(c *tc.C) {
	result := convertLegacySequenceName("task")
	c.Assert(result, tc.Equals, operation.OperationSequenceNamespace.String())
}

func (s *importSuite) TestConvertLegacySequenceNameStorage(c *tc.C) {
	result := convertLegacySequenceName("stores")
	c.Assert(result, tc.Equals, "storage")
}

func (s *importSuite) TestConvertLegacySequenceNameApplication(c *tc.C) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "application-myapp",
			expected: "application_myapp",
		},
		{
			input:    "application-my-cool-app",
			expected: "application_my-cool-app",
		},
		{
			input:    "application-ubuntu",
			expected: "application_ubuntu",
		},
		{
			input:    "application-a",
			expected: "application_a",
		},
	}

	for _, test := range tests {
		result := convertLegacySequenceName(test.input)
		c.Check(result, tc.Equals, test.expected,
			tc.Commentf("converting %q", test.input))
	}
}

func (s *importSuite) TestConvertLegacySequenceNameContainer(c *tc.C) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "machine0lxdContainer",
			expected: "machine_container_0",
		},
		{
			input:    "machine1kvmContainer",
			expected: "machine_container_1",
		},
		{
			input:    "machine10lxdContainer",
			expected: "machine_container_10",
		},
		{
			input:    "machine1/lxd/0lxdContainer",
			expected: "machine_container_1/lxd/0",
		},
		{
			input:    "machine1/lxd/0kvmContainer",
			expected: "machine_container_1/lxd/0",
		},
		{
			input:    "machine0/kvm/0lxdContainer",
			expected: "machine_container_0/kvm/0",
		},
	}

	for _, test := range tests {
		result := convertLegacySequenceName(test.input)
		c.Check(result, tc.Equals, test.expected,
			tc.Commentf("converting %q", test.input))
	}
}

func (s *importSuite) TestConvertContainerSequenceNameInvalid(c *tc.C) {
	// These should not match the container pattern and return empty string.
	tests := []string{
		"machineContainer",        // No parent ID or container type
		"machine0Container",       // No container type
		"0lxdContainer",           // Missing "machine" prefix
		"machine0lxd",             // Missing "Container" suffix
		"somethingElseContainer",  // Wrong prefix
		"machine0dockerContainer", // Invalid container type
		"machine0LXDContainer",    // Wrong case for container type
	}

	for _, name := range tests {
		result := convertContainerSequenceName(name)
		c.Check(result, tc.Equals, "",
			tc.Commentf("invalid container sequence %q should return empty string", name))
	}
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() importOperation {
	return importOperation{
		service: s.importService,
	}
}
