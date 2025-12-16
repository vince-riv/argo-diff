package gendiff

import (
	"strings"
	"testing"
)

func TestUnifiedDiff(t *testing.T) {
	tests := []struct {
		name     string
		srcFile  string
		destFile string
		from     string
		to       string
		wantDiff bool
	}{
		{
			name:     "identical strings",
			srcFile:  "file1.txt",
			destFile: "file2.txt",
			from:     "hello world\n",
			to:       "hello world\n",
			wantDiff: false,
		},
		{
			name:     "simple change",
			srcFile:  "old.txt",
			destFile: "new.txt",
			from:     "hello world\n",
			to:       "hello universe\n",
			wantDiff: true,
		},
		{
			name:     "added line",
			srcFile:  "before.txt",
			destFile: "after.txt",
			from:     "line1\nline2\n",
			to:       "line1\nline2\nline3\n",
			wantDiff: true,
		},
		{
			name:     "removed line",
			srcFile:  "before.txt",
			destFile: "after.txt",
			from:     "line1\nline2\nline3\n",
			to:       "line1\nline3\n",
			wantDiff: true,
		},
		{
			name:     "multiline change",
			srcFile:  "config-old.yaml",
			destFile: "config-new.yaml",
			from: `apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  replicas: 2
`,
			to: `apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  replicas: 3
`,
			wantDiff: true,
		},
		{
			name:     "empty strings",
			srcFile:  "empty1.txt",
			destFile: "empty2.txt",
			from:     "",
			to:       "",
			wantDiff: false,
		},
		{
			name:     "from empty to content",
			srcFile:  "empty.txt",
			destFile: "filled.txt",
			from:     "",
			to:       "new content\n",
			wantDiff: true,
		},
		{
			name:     "from content to empty",
			srcFile:  "filled.txt",
			destFile: "empty.txt",
			from:     "old content\n",
			to:       "",
			wantDiff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UnifiedDiff(tt.srcFile, tt.destFile, tt.from, tt.to)

			if tt.wantDiff {
				// Should contain diff markers
				if result == "" {
					t.Errorf("UnifiedDiff() expected a diff but got empty string")
				}
				// Unified diff should contain file headers
				if !strings.Contains(result, "---") || !strings.Contains(result, "+++") {
					t.Errorf("UnifiedDiff() expected unified diff format with --- and +++ headers, got:\n%s", result)
				}
			} else {
				// Should be empty for identical content
				if result != "" {
					t.Errorf("UnifiedDiff() expected empty string for identical content, got:\n%s", result)
				}
			}
		})
	}
}

func TestUnifiedDiffFormat(t *testing.T) {
	from := "line1\nline2\nline3\n"
	to := "line1\nline2 modified\nline3\n"

	result := UnifiedDiff("original.txt", "modified.txt", from, to)

	// Check for unified diff format elements
	if !strings.Contains(result, "--- original.txt") {
		t.Errorf("Expected '--- original.txt' in diff header")
	}
	if !strings.Contains(result, "+++ modified.txt") {
		t.Errorf("Expected '+++ modified.txt' in diff header")
	}
	if !strings.Contains(result, "-line2") {
		t.Errorf("Expected removed line marker '-line2' in diff")
	}
	if !strings.Contains(result, "+line2 modified") {
		t.Errorf("Expected added line marker '+line2 modified' in diff")
	}
}

func TestUnifiedDiffKubernetesManifest(t *testing.T) {
	// Real-world test with Kubernetes manifest changes
	oldManifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 2
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`

	newManifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.16.0
        ports:
        - containerPort: 80
`

	result := UnifiedDiff("deployment-old.yaml", "deployment-new.yaml", oldManifest, newManifest)

	// Should show the replica count change
	if !strings.Contains(result, "-  replicas: 2") {
		t.Errorf("Expected to see old replica count in diff")
	}
	if !strings.Contains(result, "+  replicas: 3") {
		t.Errorf("Expected to see new replica count in diff")
	}

	// Should show the image version change
	if !strings.Contains(result, "-        image: nginx:1.14.2") {
		t.Errorf("Expected to see old image version in diff")
	}
	if !strings.Contains(result, "+        image: nginx:1.16.0") {
		t.Errorf("Expected to see new image version in diff")
	}
}
