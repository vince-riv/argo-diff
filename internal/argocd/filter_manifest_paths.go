package argocd

import (
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// containsGlob returns true if the pattern contains glob meta characters.
func containsGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// matchChangedFiles checks if any of the changed files match one of the given patterns.
func matchChangedFiles(changedFiles []string, patterns []string) bool {
	for _, file := range changedFiles {
		// changed files shouldn't have absolute paths, but we'll trim / to be safe
		if filepath.IsAbs(file) {
			file = strings.TrimPrefix(file, "/")
		}
		for _, pattern := range patterns {
			log.Trace().Msgf("matchChangedFiles(): matching files %s to pattern %s", file, pattern)
			if containsGlob(pattern) {
				// filepath.Match expects the pattern to match the entire name.
				if ok, err := filepath.Match(pattern, file); err == nil && ok {
					return true
				} else if err != nil {
					log.Warn().Err(err).Msgf("failed to call filepath.Match(%s, %s)", pattern, file)
				}
			} else {
				// For a non-glob pattern, treat it as a directory prefix.
				dirPrefix := pattern
				if !strings.HasSuffix(dirPrefix, string(filepath.Separator)) {
					dirPrefix += string(filepath.Separator)
				}
				// Clean paths to avoid mismatches.
				cleanFile := filepath.Clean(file)
				cleanPrefix := filepath.Clean(dirPrefix)
				// Check if the changed file is under the directory.
				if strings.HasPrefix(cleanFile, cleanPrefix) {
					return true
				}
			}
		}
	}
	return false
}

// FilterApplications returns a list of Application objects whose annotation-based manifest-generate-paths
// or default source path (if the annotation is absent) match one or more of the changed files.
// It iterates through each source returned by the built-in GetSources() method.
func FilterApplicationsByPath(apps []Application, changedFiles []string) []Application {
	var matchedApps []Application

	for _, app := range apps {
		annotations := app.GetAnnotations()
		var manifestPaths string
		var ok bool
		if manifestPaths, ok = annotations["argocd.argoproj.io/manifest-generate-paths"]; !ok {
			// if the app does not have this annotation, include it in the results
			matchedApps = append(matchedApps, app)
			continue
		}
		// Treat "/" as "no annotation" - include the app without path filtering
		if strings.TrimSpace(manifestPaths) == "/" {
			matchedApps = append(matchedApps, app)
			continue
		}

		// Get all sources from the Application.
		sources := app.Spec.GetSources()
		matched := false

		for _, source := range sources {
			var patterns []string

			if manifestPaths != "" {
				// Split the annotation on semicolons and build full patterns.
				parts := strings.Split(manifestPaths, ";")
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p == "" {
						continue
					}
					var fullPattern string
					if !filepath.IsAbs(p) {
						// If p is not an absolute path, join it with the source's path.
						if p == "." {
							fullPattern = source.Path
						} else if strings.HasPrefix(p, "./") {
							fullPattern = filepath.Join(source.Path, strings.TrimPrefix(p, "./"))
						} else {
							fullPattern = filepath.Join(source.Path, p)
						}
					} else {
						fullPattern = strings.TrimPrefix(p, "/")
					}
					patterns = append(patterns, fullPattern)
				}
			} else {
				// Empty annotation, include it in the results
				matched = true
			}

			if matchChangedFiles(changedFiles, patterns) {
				matched = true
				break // No need to check further sources for this Application.
			}
		}

		if matched {
			matchedApps = append(matchedApps, app)
		}
	}

	return matchedApps
}
