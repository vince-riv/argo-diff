package gendiff

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/rs/zerolog/log"

	"gopkg.in/yaml.v2"
)

// Data structure for a kubernetes manifest and an associated diff
type K8sYaml struct {
	ApiVersion string
	Kind       string
	Name       string
	Namespace  string
	Filename   string
	YamlStr    string
	DiffStr    string
}

// Returns a list of K8sYaml (which represent differing kubernetes manifests/resources) based on a list
// of current manifests in json [from parameter] and predicted manifests in json [to parameter]
func K8sAppDiff(from, to []string, fromName, toName string) ([]K8sYaml, error) {
	fromM := make(map[string]K8sYaml)
	toM := make(map[string]K8sYaml)
	var files []string
	var diffs []K8sYaml

	for idx, m := range from {
		log.Trace().Msgf("K8sAppDif() - from[%d]: %s", idx, m)
		k, err := k8sJsonToYaml(m)
		if err != nil {
			return diffs, err
		}
		fromM[k.Filename] = k
		files = append(files, k.Filename)
	}

	for idx, m := range to {
		log.Trace().Msgf("K8sAppDif() - to[%d]: %s", idx, m)
		k, err := k8sJsonToYaml(m)
		if err != nil {
			return diffs, err
		}
		toM[k.Filename] = k
		if _, exists := fromM[k.Filename]; !exists {
			files = append(files, k.Filename)
		}
	}

	sort.Strings(files)

	for _, f := range files {
		diff := unifiedDiff(fromName+".yaml", toName+".yaml", fromM[f].YamlStr, toM[f].YamlStr)
		if diff != "" {
			k := toM[f]
			if k.Filename == "" {
				k = fromM[f]
			}
			k.DiffStr = diff
			diffs = append(diffs, k)
		}
	}
	return diffs, nil
}

// Extracts kubernetes resource metadata and generates a fake filename based on it
func manifestFilename(jsonObj map[string]interface{}) (string, string, string, string, string, error) {
	var apiVersion, kind, name, ns string

	if val, ok := jsonObj["apiVersion"].(string); ok {
		apiVersion = val
	} else {
		return apiVersion, kind, name, ns, "", fmt.Errorf("apiVersion missing in jsonObj")
	}
	if val, ok := jsonObj["kind"].(string); ok {
		kind = val
	} else {
		return apiVersion, kind, name, ns, "", fmt.Errorf("kind missing in jsobObj")
	}

	if metadata, ok := jsonObj["metadata"].(map[string]interface{}); ok {
		if val, ok := metadata["name"].(string); ok {
			name = val
		} else {
			return apiVersion, kind, name, ns, "", fmt.Errorf("name missing from jsonObj.metadata")
		}
		if val, ok := metadata["namespace"].(string); ok {
			ns = val
		}
	} else {
		return apiVersion, kind, name, ns, "", fmt.Errorf("metadata missing from jsonObj")
	}

	re := regexp.MustCompile("[^a-zA-Z0-9_.-]+")
	if ns == "" {
		return apiVersion, kind, name, ns, re.ReplaceAllString(apiVersion+"_"+kind+"_"+name, "_"), nil
	}
	return apiVersion, kind, name, ns, re.ReplaceAllString(ns+"-"+apiVersion+"_"+kind+"_"+name, "_"), nil
}

// Converts a Kubernetets manifests in json to a K8sYaml
func k8sJsonToYaml(JsonString string) (K8sYaml, error) {
	var jsonObj map[string]interface{}
	k := K8sYaml{}

	err := json.Unmarshal([]byte(JsonString), &jsonObj)
	if err != nil {
		return k, err
	}

	k.ApiVersion, k.Kind, k.Name, k.Namespace, k.Filename, err = manifestFilename(jsonObj)
	if err != nil {
		return k, err
	}

	yamlObj, err := yaml.Marshal(jsonObj)
	if err != nil {
		return k, err
	}
	k.YamlStr = string(yamlObj)

	return k, nil
}

// Produces a unified diff of two strings
func unifiedDiff(srcFile, destFile, from, to string) string {
	edits := myers.ComputeEdits(span.URIFromPath(srcFile), from, to)
	diff := fmt.Sprint(gotextdiff.ToUnified(srcFile, destFile, from, edits))
	log.Trace().Msgf("unifiedDiff(%s, %s): %s", srcFile, destFile, diff)
	return diff
}
