package gendiff

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/rs/zerolog/log"

	"gopkg.in/yaml.v2"
)

type K8sYaml struct {
	Filename string
	YamlStr  string
}

func K8sAppDiff(from, to []string) (string, error) {
	fromM := make(map[string]string)
	toM := make(map[string]string)
	var files, diffs []string

	for idx, m := range from {
		log.Trace().Msgf("K8sAppDif() - from[%d]: %s", idx, m)
		k, err := k8sJsonToYaml(m)
		if err != nil {
			return "", err
		}
		fromM[k.Filename] = k.YamlStr
		files = append(files, k.Filename)
	}

	for idx, m := range to {
		log.Trace().Msgf("K8sAppDif() - to[%d]: %s", idx, m)
		k, err := k8sJsonToYaml(m)
		if err != nil {
			return "", err
		}
		toM[k.Filename] = k.YamlStr
		if _, exists := fromM[k.Filename]; !exists {
			files = append(files, k.Filename)
		}
	}

	sort.Strings(files)

	for _, f := range files {
		diff := unifiedDiff(f+".yaml", f+"-new.yaml", fromM[f], toM[f])
		if diff != "" {
			diffs = append(diffs, diff)
		}
	}
	return strings.Join(diffs, ""), nil
}

func manifestFilename(jsonObj map[string]interface{}) (string, error) {
	var apiVersion, kind, name, ns string

	if val, ok := jsonObj["apiVersion"].(string); ok {
		apiVersion = val
	} else {
		return "", fmt.Errorf("apiVersion missing in jsonObj")
	}
	if val, ok := jsonObj["kind"].(string); ok {
		kind = val
	} else {
		return "", fmt.Errorf("kind missing in jsobObj")
	}

	if metadata, ok := jsonObj["metadata"].(map[string]interface{}); ok {
		if val, ok := metadata["name"].(string); ok {
			name = val
		} else {
			return "", fmt.Errorf("name missing from jsonObj.metadata")
		}
		if val, ok := metadata["namespace"].(string); ok {
			ns = val
		}
	} else {
		return "", fmt.Errorf("metadata missing from jsonObj")
	}

	re := regexp.MustCompile("[^a-zA-Z0-9_.-]+")
	if ns == "" {
		return re.ReplaceAllString(apiVersion+"_"+kind+"_"+name, "_"), nil
	}
	return re.ReplaceAllString(ns+"-"+apiVersion+"_"+kind+"_"+name, "_"), nil
}

func k8sJsonToYaml(JsonString string) (K8sYaml, error) {
	var jsonObj map[string]interface{}
	retval := K8sYaml{
		Filename: "",
		YamlStr:  "",
	}

	err := json.Unmarshal([]byte(JsonString), &jsonObj)
	if err != nil {
		return retval, err
	}

	retval.Filename, err = manifestFilename(jsonObj)
	if err != nil {
		return retval, err
	}

	yamlObj, err := yaml.Marshal(jsonObj)
	if err != nil {
		return retval, err
	}
	retval.YamlStr = string(yamlObj)

	return retval, nil
}

func unifiedDiff(srcFile, destFile, from, to string) string {
	edits := myers.ComputeEdits(span.URIFromPath(srcFile), from, to)
	diff := fmt.Sprint(gotextdiff.ToUnified(srcFile, destFile, from, edits))
	log.Trace().Msgf("unifiedDiff(%s, %s): %s", srcFile, destFile, diff)
	return diff
}
