package ignorable

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("ignorable_testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func setActive(t *testing.T) {
	t.Helper()
	t.Setenv("ARGO_DIFF_COMMENT_COLLAPSE", "auto")
	t.Setenv("ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE", "")
	t.Setenv(EnvRegexes, "")
	t.Setenv(EnvExcludeKinds, "")
}

func TestParseRegexes(t *testing.T) {
	long := strings.Repeat("a", maxEntryLen+1)
	var many []string
	for i := range maxListEntries + 2 {
		many = append(many, "a"+strconv.Itoa(i))
	}
	tests := []struct {
		name      string
		raw       string
		want      []string
		warnings  int
		wantEmpty bool
	}{
		{"blank lines and crlf", "foo\r\n\r\n  bar  \r\n", []string{"foo", "bar"}, 0, false},
		{"comments", "# note\nfoo\n  # another\nbar", []string{"foo", "bar"}, 0, false},
		{"escaped hash", `\#foo` + "\n" + `[#]bar`, []string{`\#foo`, `[#]bar`}, 0, false},
		{"invalid skipped", "foo(\nbar", []string{"bar"}, 1, false},
		{"too long", long + "\nbar", []string{"bar"}, 1, false},
		{"too many", strings.Join(many, "\n"), many[:maxListEntries], 2, false},
		{"sentinel", "[]", nil, 0, true},
		{"sentinel with spaces", " [] \n", nil, 0, true},
		{"all invalid", "foo(\n(bar", nil, 2, false},
		{"comma is not a separator", "a,b|c", []string{"a,b|c"}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, warns, empty := parseRegexes(tt.raw, "test")
			if empty != tt.wantEmpty || len(warns) != tt.warnings {
				t.Fatalf("empty=%v warnings=%v, want empty=%v warnings=%d", empty, warns, tt.wantEmpty, tt.warnings)
			}
			var got []string
			for _, re := range res {
				got = append(got, re.String())
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseGroupKinds(t *testing.T) {
	tests := []struct {
		raw      string
		want     []GroupKind
		warnings []string // substrings
		empty    bool
	}{
		{raw: "argoproj.io/*", want: []GroupKind{{"argoproj.io", "*"}}},
		{raw: "rbac.authorization.k8s.io/ClusterRole", want: []GroupKind{{"rbac.authorization.k8s.io", "clusterrole"}}},
		{raw: "Secret", want: []GroupKind{{"", "secret"}}},
		{raw: "/Secret", want: []GroupKind{{"", "secret"}}},
		{raw: "Secret, apps/Deployment\n# c, d\nargoproj.io/*", want: []GroupKind{{"", "secret"}, {"apps", "deployment"}, {"argoproj.io", "*"}}},
		{raw: "*", warnings: []string{"bare *"}},
		{raw: "*/Foo", warnings: []string{"group"}},
		{raw: "/*", warnings: []string{"needs a group"}},
		{raw: "a/b/c", warnings: []string{"more than one"}},
		{raw: "argoproj.io", warnings: []string{"argoproj.io/*"}},
		{raw: "argoproj.io/", warnings: []string{"missing kind"}},
		{raw: "[]", empty: true},
		{raw: " [] ", empty: true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			res, warns, empty := parseGroupKinds(tt.raw, "test")
			if empty != tt.empty || len(warns) != len(tt.warnings) {
				t.Fatalf("empty=%v warnings=%v", empty, warns)
			}
			for i, w := range tt.warnings {
				if !strings.Contains(warns[i], w) {
					t.Errorf("warning %q lacks %q", warns[i], w)
				}
			}
			if len(res) != len(tt.want) {
				t.Fatalf("got %v, want %v", res, tt.want)
			}
			for i := range res {
				if res[i] != tt.want[i] {
					t.Errorf("got %v, want %v", res, tt.want)
				}
			}
		})
	}
}

func TestGroupKindMatch(t *testing.T) {
	gk, _ := parseGroupKind("argoproj.io/*")
	if !gk.matches("ArgoProj.io", "Application") {
		t.Error("want case-insensitive match")
	}
	if gk.matches("foo.argoproj.io", "Application") {
		t.Error("foo.argoproj.io must not match argoproj.io/*")
	}
	cr, _ := parseGroupKind("clusterrole")
	if cr.matches("rbac.authorization.k8s.io", "ClusterRole") {
		t.Error("a bare kind means the core group")
	}
}

func TestLoadGlobal(t *testing.T) {
	t.Run("inactive parses nothing", func(t *testing.T) {
		t.Setenv("ARGO_DIFF_COMMENT_COLLAPSE", "expanded")
		t.Setenv(EnvRegexes, "foo(")
		g := LoadGlobal()
		if g.Active || g.Regexes != nil || g.Exclude != nil || g.Warnings != nil {
			t.Errorf("got %+v", g)
		}
	})
	t.Run("inactive via flag", func(t *testing.T) {
		setActive(t)
		t.Setenv("ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE", "false")
		if LoadGlobal().Active {
			t.Error("want inactive")
		}
	})
	t.Run("defaults", func(t *testing.T) {
		setActive(t)
		g := LoadGlobal()
		if !g.Active || len(g.Regexes) != len(DefaultRegexes) || len(g.Exclude) != 1 || g.Exclude[0] != (GroupKind{"argoproj.io", "*"}) {
			t.Errorf("got %+v", g)
		}
	})
	t.Run("replaces defaults", func(t *testing.T) {
		setActive(t)
		t.Setenv(EnvRegexes, "^foo\n^bar")
		t.Setenv(EnvExcludeKinds, "Secret")
		g := LoadGlobal()
		if len(g.Regexes) != 2 || len(g.Exclude) != 1 || g.Exclude[0].Kind != "secret" {
			t.Errorf("got %+v", g)
		}
	})
	t.Run("sentinel", func(t *testing.T) {
		setActive(t)
		t.Setenv(EnvRegexes, "[]")
		t.Setenv(EnvExcludeKinds, "[]")
		g := LoadGlobal()
		if len(g.Regexes) != 0 || len(g.Exclude) != 0 || len(g.Warnings) != 0 {
			t.Errorf("got %+v", g)
		}
	})
	t.Run("all invalid is empty, not defaults", func(t *testing.T) {
		setActive(t)
		t.Setenv(EnvRegexes, "foo(")
		t.Setenv(EnvExcludeKinds, "*")
		g := LoadGlobal()
		if len(g.Regexes) != 0 || len(g.Exclude) != 0 || len(g.Warnings) != 2 {
			t.Errorf("got %+v", g)
		}
	})
}

func TestForApp(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		setActive(t)
		g := LoadGlobal()
		for _, tt := range []struct {
			v      string
			active bool
			warns  int
		}{{"false", false, 0}, {"true", true, 0}, {"nope", true, 1}} {
			p, w := g.ForApp(map[string]string{AnnotationFlag: tt.v})
			if p.Active != tt.active || len(w) != tt.warns {
				t.Errorf("%s: active=%v warnings=%v", tt.v, p.Active, w)
			}
		}
	})
	t.Run("nil annotations", func(t *testing.T) {
		setActive(t)
		p, w := LoadGlobal().ForApp(nil)
		if !p.Active || len(w) != 0 || !p.Ignorable("", "ConfigMap", fixture(t, "helm-labels-only.diff")) {
			t.Error("nil annotations should leave the global policy")
		}
	})
	t.Run("inactive global ignores annotations", func(t *testing.T) {
		t.Setenv("ARGO_DIFF_COMMENT_COLLAPSE", "expanded")
		p, w := LoadGlobal().ForApp(map[string]string{AnnotationRegexes: "x(", AnnotationFlag: "true"})
		if p.Active || len(w) != 0 {
			t.Error("want inactive, no warnings")
		}
	})
	t.Run("regexes append", func(t *testing.T) {
		setActive(t)
		p, _ := LoadGlobal().ForApp(map[string]string{AnnotationRegexes: `^\s*image:\s`})
		if !p.Ignorable("apps", "Deployment", "-  image: a:1\n+  image: a:2\n+    helm.sh/chart: x-1.2.3\n") {
			t.Error("want global and app regexes combined")
		}
	})
	t.Run("global sentinel", func(t *testing.T) {
		setActive(t)
		t.Setenv(EnvRegexes, "[]")
		g := LoadGlobal()
		diff := fixture(t, "helm-labels-only.diff")
		if p, _ := g.ForApp(nil); p.Ignorable("", "ConfigMap", diff) {
			t.Error("app without annotation must not be opted in")
		}
		p, _ := g.ForApp(map[string]string{AnnotationRegexes: "version:\n^\\s*helm\\.sh/chart:"})
		if !p.Ignorable("", "ConfigMap", diff) {
			t.Error("annotation regexes should opt the app in")
		}
	})
	t.Run("include kinds", func(t *testing.T) {
		setActive(t)
		g := LoadGlobal()
		diff := fixture(t, "argoproj-application-labels.diff")
		for _, tt := range []struct {
			include string
			want    bool
		}{
			{"", false},
			{"argoproj.io/Application", true},
			{"argoproj.io/*", true},
			{"argoproj.io/ApplicationSet", false},
		} {
			ann := map[string]string{}
			if tt.include != "" {
				ann[AnnotationIncludeKinds] = tt.include
			}
			p, _ := g.ForApp(ann)
			if got := p.Ignorable("argoproj.io", "Application", diff); got != tt.want {
				t.Errorf("include %q: got %v", tt.include, got)
			}
		}
		_, w := g.ForApp(map[string]string{AnnotationIncludeKinds: "argoproj.io"})
		if len(w) != 1 {
			t.Errorf("want one warning, got %v", w)
		}
	})
}

func TestIgnorableFixtures(t *testing.T) {
	setActive(t)
	p, _ := LoadGlobal().ForApp(nil)
	tests := []struct {
		file, group, kind string
		want              bool
	}{
		{"helm-labels-only.diff", "", "ConfigMap", true},
		{"legacy-chart-label.diff", "apps", "Deployment", true},
		{"subchart-label-only.diff", "apps", "DaemonSet", true},
		{"workload-image-bump.diff", "apps", "DaemonSet", false},
		{"crd-controller-gen-only.diff", "apiextensions.k8s.io", "CustomResourceDefinition", true},
		{"crd-schema-change.diff", "apiextensions.k8s.io", "CustomResourceDefinition", false},
		{"argoproj-application-labels.diff", "argoproj.io", "Application", false},
		{"helm-labels-only.diff", "foo.argoproj.io", "Thing", true},
		{"helm-labels-only.diff", "rbac.authorization.k8s.io", "ClusterRole", true},
	}
	for _, tt := range tests {
		t.Run(tt.file+"/"+tt.group+"/"+tt.kind, func(t *testing.T) {
			if got := p.Ignorable(tt.group, tt.kind, fixture(t, tt.file)); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("inactive", func(t *testing.T) {
		t.Setenv("ARGO_DIFF_COMMENT_COLLAPSE", "collapsed")
		off, _ := LoadGlobal().ForApp(nil)
		for _, f := range []string{"helm-labels-only.diff", "legacy-chart-label.diff", "crd-controller-gen-only.diff"} {
			if off.Ignorable("", "ConfigMap", fixture(t, f)) {
				t.Errorf("%s: inactive policy must not fold", f)
			}
		}
	})
}

func TestIgnorableExcludeOverrides(t *testing.T) {
	diff := fixture(t, "helm-labels-only.diff")
	setActive(t)
	t.Run("global exclude is empty", func(t *testing.T) {
		t.Setenv(EnvExcludeKinds, "[]")
		p, _ := LoadGlobal().ForApp(nil)
		if !p.Ignorable("argoproj.io", "Application", fixture(t, "argoproj-application-labels.diff")) {
			t.Error("want true")
		}
	})
	t.Run("operator added ClusterRole", func(t *testing.T) {
		t.Setenv(EnvExcludeKinds, "argoproj.io/*,rbac.authorization.k8s.io/ClusterRole")
		g := LoadGlobal()
		p, _ := g.ForApp(nil)
		if p.Ignorable("rbac.authorization.k8s.io", "ClusterRole", diff) {
			t.Error("excluded kind must not fold")
		}
		p, _ = g.ForApp(map[string]string{AnnotationIncludeKinds: "rbac.authorization.k8s.io/ClusterRole"})
		if !p.Ignorable("rbac.authorization.k8s.io", "ClusterRole", diff) {
			t.Error("include must lift the exclusion")
		}
	})
}

func TestIgnorableDiffShapes(t *testing.T) {
	setActive(t)
	p, _ := LoadGlobal().ForApp(nil)
	tests := []struct {
		name string
		diff string
		want bool
	}{
		{"empty", "", false},
		{"headers only", "--- a.yaml\t2026\n+++ b.yaml\t2026\n", false},
		{"context only", "@@ -1 +1 @@\n   foo: bar\n", false},
		{"removed document separator", "-    helm.sh/chart: a-1.2.3\n-----\n", false},
		{"chart without semver", "-  chart: my-chart\n+  chart: my-chart2\n", false},
		{"child application source chart", "-    chart: kube-prometheus-stack\n+    chart: kube-prometheus-stack-x\n", false},
		{"quoted version", "-  app.kubernetes.io/version: \"1.2.2\"\n+  app.kubernetes.io/version: \"1.2.3\"\n", true},
		{"quoted chart", "-  helm.sh/chart: \"foo-1.2.2\"\n+  helm.sh/chart: \"foo-1.2.3\"\n", true},
		{"keel chart", "-  chart: keel-v1.2.2\n+  chart: keel-v1.2.3\n", true},
		{"deep indent", "-            app.kubernetes.io/version: 1.2.2\n+            app.kubernetes.io/version: 1.2.3\n", true},
		{"mixed", "-  app.kubernetes.io/version: 1\n+  app.kubernetes.io/version: 2\n+  replicas: 3\n", false},
		{"no newline marker", "-  app.kubernetes.io/version: 1\n\\ No newline at end of file\n+  app.kubernetes.io/version: 2\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.Ignorable("", "ConfigMap", tt.diff); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
