package gendiff

import (
	"encoding/json"
	"fmt"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"

	"gopkg.in/yaml.v2"
)

func jsonToYaml(JsonString string) (string, error) {
	var jsonObj map[string]interface{}

	err := json.Unmarshal([]byte(JsonString), &jsonObj)
	if err != nil {
		return "", err
	}

	yamlObj, err := yaml.Marshal(jsonObj)
	if err != nil {
		return "", err
	}

	return string(yamlObj), nil
}

func unifiedDiff(srcFile, destFile, from, to string) (string) {
	edits := myers.ComputeEdits(span.URIFromPath(srcFile), from, to)
	diff := fmt.Sprint(gotextdiff.ToUnified(srcFile, destFile, from, edits))
	return diff
}
