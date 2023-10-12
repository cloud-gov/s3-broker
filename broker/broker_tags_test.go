package broker

import (
	"reflect"
	"testing"
)

func TestGetBucketTags(t *testing.T) {
	tags := getBucketTags("Created", "abc1", "abc2", "abc3", "abc4", "abc5")
	delete(tags, "Created at")

	expectedTags := map[string]string{
		"Owner":             "Cloud Foundry",
		"Created by":        "AWS S3 Service Broker",
		"Service GUID":      "abc1",
		"Plan GUID":         "abc2",
		"Organization GUID": "abc3",
		"Space GUID":        "abc4",
		"Instance GUID":     "abc5",
	}

	if !reflect.DeepEqual(tags, expectedTags) {
		t.Errorf("expected: %s, got: %s", expectedTags, tags)
	}
}
