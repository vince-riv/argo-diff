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
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"sigs.k8s.io/yaml"
)

// appNamespaceManifestsMinVersion is the minimum argocd CLI client version
// that accepts --app-namespace on `argocd app manifests`. `argocd app diff`
// has accepted it since v2.5.0 (below minVersion), so only `app manifests`
// needs gating.
const appNamespaceManifestsMinVersion = "3.5.0"

var (
	httpBearerToken       string
	commonCliArgv         []string
	envArgoCdOpts         string
	appDiffServerSideDiff string
)

func init() {
	serverAddr := os.Getenv("ARGOCD_SERVER_ADDR")
	httpBearerToken = os.Getenv("ARGOCD_AUTH_TOKEN")
	envArgoCdOpts = os.Getenv("ARGOCD_OPTS")
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
	// check to see if we need to enable/disable server-side diff for app diff commands
	envAppDiffServerSide := strings.ToLower(os.Getenv("ARGOCD_APP_DIFF_SERVER_SIDE_DIFF"))
	if envAppDiffServerSide == "true" || envAppDiffServerSide == "false" {
		appDiffServerSideDiff = envAppDiffServerSide
	} else if envAppDiffServerSide != "" {
		log.Warn().Msgf("Invalid value for ARGOCD_APP_DIFF_SERVER_SIDE_DIFF: %s; must be 'true' or 'false'", envAppDiffServerSide)
	}
}

func argocdCmdFromEnv() string {
	cmdName := os.Getenv("ARGOCD_CLI_CMD_NAME")
	if cmdName != "" {
		return cmdName
	}
	return "argocd"
}

// function that uses log.Trace to log all environment variables for debugging. redact ARGOCD_AUTH_TOKEN and GITHUB_TOKEN, if set
func logTraceCommandEnv(cmd *exec.Cmd) {
	for _, envVar := range cmd.Env {
		if strings.HasPrefix(envVar, "ARGOCD_AUTH_TOKEN=") || strings.HasPrefix(envVar, "GITHUB_TOKEN=") {
			log.Trace().Msgf("%s=REDACTED", strings.SplitN(envVar, "=", 2)[0])
		} else {
			log.Trace().Msg(envVar)
		}
	}
}

// Wrapper around argocd cli; returns raw output in []bytes
// Set as variable so it can be mocked in tests
var execArgoCdCli = func(ctx context.Context, args []string) ([]byte, error) {
	argocdCmdName := argocdCmdFromEnv()
	log.Info().Msgf("Executing %s with args %s", argocdCmdName, strings.Join(args, " "))
	// slices.Concat always allocates a fresh backing array; a plain append(commonCliArgv, args...)
	// can write into commonCliArgv's spare capacity and corrupt other concurrent callers' argv.
	argv := slices.Concat(commonCliArgv, args)
	cmd := exec.CommandContext(ctx, argocdCmdName, argv...)
	cmd.Env = append(cmd.Environ(), "KUBECTL_EXTERNAL_DIFF=diff -u")
	if envArgoCdOpts != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ARGOCD_OPTS=%s", envArgoCdOpts))
	}
	logTraceCommandEnv(cmd)
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

// extractVersionField scans output for a line of the form "<prefix> <value>"
// (as printed by `argocd version`/`argocd version --client`, e.g.
// "argocd: v2.13.1+abcdef") and returns value with any "+build" suffix
// trimmed. ok is false if no such line is found.
func extractVersionField(output []byte, prefix string) (value string, ok bool) {
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if strings.HasPrefix(trimmed, prefix) {
			parts := strings.SplitN(trimmed, " ", 2)
			if len(parts) == 2 {
				value = strings.TrimSpace(parts[1])
				value = strings.Split(value, "+")[0]
				return value, true
			}
		}
	}
	return "", false
}

// ParseArgoCDVersion extracts the client and server version from the output of "argocd version".
// It trims everything after the '+' sign, including the sign itself.
func parseArgoCDVersion(output []byte) (clientVersion, serverVersion string, err error) {
	clientVersion, _ = extractVersionField(output, "argocd:")
	serverVersion, _ = extractVersionField(output, "argocd-server:")
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

// parseArgoCDClientVersion extracts the client version from the output of
// "argocd version --client", which prints only the client block (no
// "argocd-server:" line).
func parseArgoCDClientVersion(output []byte) (string, error) {
	clientVersion, ok := extractVersionField(output, "argocd:")
	if !ok {
		return "", fmt.Errorf("failed to parse client version from output")
	}
	return clientVersion, nil
}

func argocdClientVersion(ctx context.Context) (string, error) {
	// argocd version --client - inspects only the local CLI binary and makes
	// no network round-trip to the ArgoCD server, unlike argocdVersion()
	// above. This is not a substitute for ConnectivityCheck().
	output, err := execArgoCdCli(ctx, []string{"version", "--client"})
	if err != nil {
		log.Error().Err(err).Msg("argocd version --client failed")
		return "", err
	}
	return parseArgoCDClientVersion(output)
}

var (
	clientVersionMu                     sync.Mutex
	cachedSupportsManifestsAppNamespace *bool
)

// supportsManifestsAppNamespace reports whether the pinned argocd CLI is new
// enough (>=3.5.0) to accept --app-namespace on `argocd app manifests`. An
// operator can pin an older CLI via ARGOCD_CLI_CMD_NAME; without this check,
// app-of-apps discovery for apps outside the CLI's default namespace would
// pass a flag the CLI doesn't understand.
//
// The result is cached process-wide after the first successful lookup,
// since the pinned CLI's version cannot change during the process lifetime.
// A failed lookup is not cached, so a transient error is retried on the next
// call rather than permanently disabling app-namespace support.
func supportsManifestsAppNamespace(ctx context.Context) bool {
	clientVersionMu.Lock()
	defer clientVersionMu.Unlock()
	if cachedSupportsManifestsAppNamespace != nil {
		return *cachedSupportsManifestsAppNamespace
	}
	v, err := argocdClientVersion(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to determine argocd CLI client version; omitting --app-namespace from `argocd app manifests` (app-of-apps discovery will only find nested apps in the CLI's default namespace)")
		return false
	}
	supported := versionAtLeast(v, appNamespaceManifestsMinVersion)
	cachedSupportsManifestsAppNamespace = &supported
	return supported
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

func getApplicationManifests(ctx context.Context, appName, appNamespace, revision string) ([]K8sManifest, error) {
	// argocd app manifests argo-diff --revision HEAD
	args := []string{"app", "manifests", appName, "--revision", revision}
	if supportsManifestsAppNamespace(ctx) {
		args = append(args, "--app-namespace", appNamespace)
	}
	output, err := execArgoCdCli(ctx, args)
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

func diffApplication(ctx context.Context, appName string, appNamespace string, revision string, revisions []string, srcPos []int) ([]AppResource, error) {
	var appResList []AppResource
	log.Trace().Msg("diffApplication() called")
	// argocd app diff argo-diff --revision XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX [--refresh]
	// argocd app diff argo-diff --revisions XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX --source-positions 1 --revisions a.b.c --source-positions 2
	args := []string{"app", "diff", appName, "--app-namespace", appNamespace, "--revision", revision}
	if len(revisions) > 0 {
		args = []string{"app", "diff", appName, "--app-namespace", appNamespace}
		for _, rev := range revisions {
			args = append(args, "--revisions")
			args = append(args, rev)
		}
		for _, pos := range srcPos {
			args = append(args, "--source-positions")
			args = append(args, strconv.Itoa(pos))
		}
	}
	if appDiffServerSideDiff != "" {
		args = append(args, fmt.Sprintf("--server-side-diff=%s", appDiffServerSideDiff))
	}
	output, err := execArgoCdCli(ctx, args)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 && len(output) > 0 {
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
