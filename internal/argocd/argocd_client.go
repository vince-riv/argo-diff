package argocd

/*
 * Simple wrapper around argocd cli
 */

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"sigs.k8s.io/yaml"
)

var (
	httpBearerToken string
	commonCliArgv   []string
)

func init() {
	serverAddr := os.Getenv("ARGOCD_SERVER_ADDR")
	httpBearerToken = os.Getenv("ARGOCD_AUTH_TOKEN")
	insecure := strings.ToLower(os.Getenv("ARGOCD_SERVER_INSECURE")) == "true"
	plaintext := strings.ToLower(os.Getenv("ARGOCD_SERVER_PLAINTEXT")) == "true"
	grpcWeb := strings.ToLower(os.Getenv("ARGOCD_GRPC_WEB")) == "true"
	grpcWebRoot := os.Getenv("ARGOCD_GRPC_WEB_ROOT_PATH")
	if serverAddr == "" || httpBearerToken == "" {
		log.Warn().Msg("Initialized with incomplete ArgoCD server config")
	}
	commonCliArgv = append(commonCliArgv, "--server", serverAddr)
	commonCliArgv = append(commonCliArgv, "--auth-token", httpBearerToken)
	if insecure {
		commonCliArgv = append(commonCliArgv, "--insecure")
	}
	if plaintext {
		commonCliArgv = append(commonCliArgv, "--plaintext")
	}
	if grpcWeb {
		commonCliArgv = append(commonCliArgv, "--grpc-web")
	}
	if grpcWebRoot != "" {
		commonCliArgv = append(commonCliArgv, "--grpc-web-root-path", grpcWebRoot)
	}
}

func argocdCmdFromEnv() string {
	cmdName := os.Getenv("ARGOCD_CLI_CMD_NAME")
	if cmdName != "" {
		return cmdName
	}
	return "argocd"
}

// Wrapper around argocd cli; returns raw output in []bytes
// Set as variable so it can be mocked in tests
var execArgoCdCli = func(ctx context.Context, args []string) ([]byte, error) {
	argocdCmdName := argocdCmdFromEnv()
	log.Info().Msgf("Executing %s with args %s", argocdCmdName, strings.Join(args, " "))
	argv := append(commonCliArgv, args...)
	cmd := exec.CommandContext(ctx, argocdCmdName, argv...)
	cmd.Env = append(cmd.Environ(), "KUBECTL_EXTERNAL_DIFF=diff -u")
	out, err := cmd.Output()
	if err != nil {
		// log.Error().Err(err).Msgf("Failed to execute: %s ... %s", argocdCmdName, strings.Join(argv, " "))
		return out, err
	}
	return out, nil
}

func listApplications(ctx context.Context) (*ApplicationList, error) {
	var appList []Application
	var apps ApplicationList
	log.Trace().Msg("listApplications() called")
	// argocd app list
	output, err := execArgoCdCli(ctx, []string{"app", "list", "-o", "json"})
	if err != nil {
		log.Error().Err(err).Msg("Application List failed")
		return nil, err
	}
	err = json.Unmarshal(output, &appList)
	if err != nil {
		log.Error().Err(err).Msg("Decoding Application List failed")
		return nil, err
	}
	apps.Items = appList
	return &apps, nil
}

// ParseArgoCDVersion extracts the client and server version from the output of "argocd version".
// It trims everything after the '+' sign, including the sign itself.
func parseArgoCDVersion(output []byte) (clientVersion, serverVersion string, err error) {
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if strings.HasPrefix(trimmed, "argocd:") {
			// Extract client version
			parts := strings.SplitN(trimmed, " ", 2)
			if len(parts) == 2 {
				clientVersion = strings.TrimSpace(parts[1])
				clientVersion = strings.Split(clientVersion, "+")[0]
			}
		} else if strings.HasPrefix(trimmed, "argocd-server:") {
			// Extract server version
			parts := strings.SplitN(trimmed, " ", 2)
			if len(parts) == 2 {
				serverVersion = strings.TrimSpace(parts[1])
				serverVersion = strings.Split(serverVersion, "+")[0]
			}
		}
	}
	if clientVersion == "" || serverVersion == "" {
		return "", "", fmt.Errorf("failed to parse client or server version from output")
	}
	return clientVersion, serverVersion, nil
}

func argocdVersion(ctx context.Context) (string, string, error) {
	// argocd version
	output, err := execArgoCdCli(ctx, []string{"version"})
	if err != nil {
		log.Error().Err(err).Msg("argocd version failed")
		return "", "", err
	}
	return parseArgoCDVersion(output)
}

/*
func getApplication(ctx context.Context, appName string) (*Application, error) {
	var app Application
	// argocd app get argo-diff --refresh
	output, err := execArgoCdCli(ctx, []string{"app", "get", appName, "--refresh", "-o", "json"})
	if err != nil {
		log.Error().Err(err).Msgf("Get Argo application %s failed", appName)
		return nil, err
	}
	err = json.Unmarshal(output, &app)
	if err != nil {
		log.Error().Err(err).Msg("Decoding Application failed")
		return nil, err
	}
	return &app, nil
}
*/

func appManifestHelper(input []byte) ([]K8sManifest, error) {
	var manifests []K8sManifest
	yamlDocs := strings.Split(string(input), "\n---")
	for _, doc := range yamlDocs {
		if strings.TrimSpace(doc) == "" {
			continue // Skip empty documents
		}
		var manifest K8sManifest
		manifest.YamlSrc = []byte(doc)
		err := yaml.Unmarshal([]byte(doc), &manifest.Unstruct.Object)
		if err != nil {
			return manifests, err
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func getApplicationManifests(ctx context.Context, appName, revision string) ([]K8sManifest, error) {
	// argocd app manifests argo-diff --revision HEAD
	output, err := execArgoCdCli(ctx, []string{"app", "manifests", appName, "--revision", revision})
	if err != nil {
		log.Error().Err(err).Msgf("Get Argo application manifests for %s failed", appName)
		return nil, err
	}
	manifests, err := appManifestHelper(output)
	if err != nil {
		log.Error().Err(err).Msgf("Decoding yaml output failed for %s at revision %s", appName, revision)
		return manifests, err
	}
	return manifests, nil
}

func diffApplication(ctx context.Context, appName string, revision string, revisions []string, srcPos []int) ([]AppResource, error) {
	var appResList []AppResource
	log.Trace().Msg("diffApplication() called")
	// argocd app diff argo-diff --revision XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX [--refresh]
	// argocd app diff argo-diff --revisions XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX --source-positions 1 --revisions a.b.c --source-positions 2
	args := []string{"app", "diff", appName, "--revision", revision}
	if len(revisions) > 0 {
		args = []string{"app", "diff", appName}
		for _, rev := range revisions {
			args = append(args, "--revisions")
			args = append(args, rev)
		}
		for _, pos := range srcPos {
			args = append(args, "--source-positions")
			args = append(args, strconv.Itoa(pos))
		}
	}
	output, err := execArgoCdCli(ctx, args)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				if len(output) == 0 {
					output = exitErr.Stderr
				}
				log.Trace().Msgf("Application %s revision %s has changes: %s", appName, revision, output)
				for _, diffStr := range diffBytesToStr(output) {
					var appRes AppResource
					hdrStr, diffStr := extractFirstLine(diffStr)
					appRes.DiffStr = diffStr
					appRes.Group, appRes.Kind, appRes.Namespace, appRes.Name = extractKubernetesFields(hdrStr)
					appResList = append(appResList, appRes)
				}
				return appResList, nil
			} else {
				execError := fmt.Errorf("%s: %s: %s", strings.Join(args, " "), err.Error(), exitErr.Stderr)
				log.Error().Err(err).Msgf("Application diff for %s, revision %s, failed", appName, revision)
				return nil, execError
			}
		} else {
			log.Error().Err(err).Msgf("Application diff for %s, revision %s, had an unknown failure", appName, revision)
			execError := fmt.Errorf("%s: %s", strings.Join(args, " "), err.Error())
			return nil, execError
		}
	}
	log.Trace().Msgf("Application %s revision %s has no changes", appName, revision)
	return appResList, nil
}

func diffBytesToStr(input []byte) []string {
	// each resource diff has a header that looks like this:
	// ===== rbac.authorization.k8s.io/ClusterRoleBinding /loki-clusterrolebinding ======
	delimiter := []byte("\n\n=====")
	// Split the byte slice into parts
	parts := bytes.Split(input, delimiter)
	// Rebuild the strings with specific rules for first and last elements
	result := make([]string, len(parts))
	// If there's one element, don't re-construct
	if len(parts) == 1 {
		result[0] = string(parts[0])
	} else {
		for i, part := range parts {
			switch i {
			case 0:
				// First element only has "\n\n" re-appended
				result[i] = string(part) + "\n\n"
			case len(parts) - 1:
				// Last element only has "=====" re-prepended
				result[i] = "=====" + string(part)
			default:
				// Middle elements have "=====" prepended and "\n\n" appended
				result[i] = "=====" + string(part) + "\n\n"
			}
		}
	}
	return result
}

func extractFirstLine(input string) (firstLine string, remaining string) {
	input = strings.TrimLeft(input, "\n")
	// Split the string into lines
	lines := strings.SplitN(input, "\n", 2)
	if len(lines) > 0 {
		firstLine = lines[0]
	}
	if len(lines) > 1 {
		remaining = lines[1]
	}
	return
}

func extractKubernetesFields(input string) (group, kind, namespace, name string) {
	// Remove the "=====" wrapper
	trimmed := strings.TrimSpace(strings.Trim(input, "="))

	// Split the remaining string into parts
	parts := strings.Fields(trimmed)
	if len(parts) == 2 {
		// Extract group/kind and namespace/name
		groupKind := parts[0]
		namespaceName := parts[1]

		// Further split group/kind
		groupKindParts := strings.SplitN(groupKind, "/", 2)
		if len(groupKindParts) == 2 {
			group, kind = groupKindParts[0], groupKindParts[1]
		} else if len(groupKindParts) == 1 {
			kind = groupKindParts[0]
		}

		// Further split namespace/name
		namespaceNameParts := strings.SplitN(namespaceName, "/", 2)
		if len(namespaceNameParts) == 2 {
			namespace, name = namespaceNameParts[0], namespaceNameParts[1]
		} else if len(namespaceNameParts) == 1 {
			name = namespaceNameParts[0]
		}
	}
	return
}
