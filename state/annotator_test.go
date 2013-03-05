package state_test

import (
	. "launchpad.net/gocheck"
)

type annotator interface {
	Annotation(key string) string
	Annotations() map[string]string
	Refresh() error
	RemoveAnnotation(key string) error
	SetAnnotation(key, value string) error
}

func testSetAnnotation(c *C, entity annotator) {
	err := entity.SetAnnotation("mykey", "myvalue")
	c.Assert(err, IsNil)
	// The value stored in the annotator changed.
	c.Assert(entity.Annotations()["mykey"], Equals, "myvalue")
	err = entity.Refresh()
	c.Assert(err, IsNil)
	// The value stored in MongoDB changed.
	c.Assert(entity.Annotations()["mykey"], Equals, "myvalue")
}

func testMultipleSetAnnotation(c *C, entity annotator) {
	err := entity.SetAnnotation("mykey", "myvalue")
	c.Assert(err, IsNil)
	err = entity.SetAnnotation("another-key", "another-value")
	c.Assert(err, IsNil)
	annotations := entity.Annotations()
	expected := map[string]string{
		"mykey":       "myvalue",
		"another-key": "another-value",
	}
	c.Assert(annotations, DeepEquals, expected)
}

func testSetInvalidAnnotation(c *C, entity annotator) {
	err := entity.SetAnnotation("invalid.key", "myvalue")
	c.Assert(err, ErrorMatches, `invalid key "invalid.key"`)
}

func testAnnotation(c *C, entity annotator) {
	err := entity.SetAnnotation("mykey", "myvalue")
	c.Assert(err, IsNil)
	c.Assert(entity.Annotation("mykey"), Equals, "myvalue")
}

func testNonExistantAnnotation(c *C, entity annotator) {
	c.Assert(entity.Annotation("does-not-exist"), Equals, "")
}

func testRemoveAnnotation(c *C, entity annotator) {
	err := entity.SetAnnotation("mykey", "myvalue")
	c.Assert(err, IsNil)
	err = entity.RemoveAnnotation("mykey")
	c.Assert(err, IsNil)
	c.Assert(entity.Annotation("mykey"), Equals, "")
	c.Assert(entity.Annotations()["mykey"], Equals, "")
}

func testRemoveNonExistantAnnotation(c *C, entity annotator) {
	err := entity.RemoveAnnotation("does-not-exist")
	c.Assert(err, IsNil)
}

func testAnnotator(c *C, getEntity func() (annotator, error)) {
	tests := []struct {
		about string
		test  func(c *C, entity annotator)
	}{
		{
			about: "test setting an annotation",
			test:  testSetAnnotation,
		},
		{
			about: "test setting multiple annotations",
			test:  testMultipleSetAnnotation,
		},
		{
			about: "test setting an invalid annotation",
			test:  testSetInvalidAnnotation,
		},
		{
			about: "test returning annotations",
			test:  testAnnotation,
		},
		{
			about: "test returning a non existant annotation",
			test:  testNonExistantAnnotation,
		},
		{
			about: "test removing an annotation",
			test:  testRemoveAnnotation,
		},
		{
			about: "test removing a non existant annotation",
			test:  testRemoveNonExistantAnnotation,
		},
	}
	for i, t := range tests {
		c.Logf("test %d. %s", i, t.about)
		entity, err := getEntity()
		c.Assert(err, IsNil)
		t.test(c, entity)
	}

}
