package gendiff

import (
	"testing"
)

func TestJsonToYaml(t *testing.T) {
	testStr := "{\"key\": \"val1\"}"
	yamlStr, err := jsonToYaml(testStr)
	if err != nil {
		t.Error("jsonToYaml() failed")
	}
	if yamlStr != "key: val1\n" {
		t.Error("jsonToYaml() returned unexpected YAML: " + yamlStr)
	}
}

func TestUnifiedDiff(t *testing.T) {
	str1 := "key: val1\n"
	str2 := "key: val2\n"
	diff := unifiedDiff("test1.yaml", "test2.yaml", str1, str2)
	expectedDiff := "--- test1.yaml\n+++ test2.yaml\n@@ -1 +1 @@\n-key: val1\n+key: val2\n"
	// println(diff)
	if diff == "" {
		t.Error("unifiedDiff() produced empty diff")
	}
	if diff != expectedDiff {
		t.Error("unifiedDiff() produced unexpected diff")
	}
}

func TestUnifiedDiffNoDiff(t *testing.T) {
	str1 := "key: val1\n"
	str2 := "key: val1\n"
	diff := unifiedDiff("test1.yaml", "test2.yaml", str1, str2)
	if diff != "" {
		t.Error("unifiedDiff() didn't produce an empty diff")
	}
}
