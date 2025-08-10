package argocd

import (
	"encoding/json"
	"testing"
)

func TestFilterApplicationsByPath(t *testing.T) {
	var a []Application
	annotationStr := "argocd.argoproj.io/manifest-generate-paths"
	payload, _, err := readFileToByteArray(payloadAppList)
	if err != nil {
		t.Errorf("Failed to read %s: %v", payloadAppList, err)
	}
	var appList ApplicationList
	if err := json.Unmarshal(payload, &appList); err != nil {
		t.Errorf("Error decoding ApplicationList payload: %v", err)
	}
	a = appList.Items
	if err != nil {
		t.Errorf("decodeApplicationListPayload() failed: %s", err)
	}

	// no annotations set, should get the same apps back
	passThru := FilterApplicationsByPath(a, []string{"doesnt", "matter"})
	if len(passThru) != len(a) {
		t.Error("passthrough failed for FilterApplicationByPath()")
	}

	a1 := []Application{a[0]}
	a1[0].SetAnnotations(map[string]string{
		annotationStr: ".",
	})

	relativeNoMatch := FilterApplicationsByPath(a1, []string{"not_apps/manifest.yaml", "something/else.yaml"})
	if len(relativeNoMatch) != 0 {
		t.Error("relativeNoMatch failed")
	}

	relativeMatch := FilterApplicationsByPath(a1, []string{"apps/somepath/manifest.yaml", "something/else.yaml"})
	if len(relativeMatch) != 1 {
		t.Error("relativeMatch failed")
	}

	a1[0].SetAnnotations(map[string]string{
		annotationStr: "/apps",
	})

	absoluteNoMatch := FilterApplicationsByPath(a1, []string{"not_apps/manifest.yaml", "something/else.yaml"})
	if len(absoluteNoMatch) != 0 {
		t.Error("absoluteNoMatch failed")
	}

	absoluteMatch := FilterApplicationsByPath(a1, []string{"apps/somepath/manifest.yaml", "something/else.yaml"})
	if len(absoluteMatch) != 1 {
		t.Error("absoluteMatch failed")
	}

	a1[0].SetAnnotations(map[string]string{
		annotationStr: "/shared/application-*.yaml",
	})

	globNoMatch := FilterApplicationsByPath(a1, []string{"somepath/application-testing.yaml", "something/else.yaml"})
	if len(globNoMatch) != 0 {
		t.Error("globNoMatch failed")
	}

	globMatch := FilterApplicationsByPath(a1, []string{"shared/application-testing_123.yaml", "something/else.yaml"})
	if len(globMatch) != 1 {
		t.Error("globMatch failed")
	}

	a1[0].SetAnnotations(map[string]string{
		annotationStr: ".;/shared/application-*.yaml;/more/apps/",
	})

	mixedNoMatch := FilterApplicationsByPath(a1, []string{"somepath/application-testing.yaml", "something/else.yaml", "more/notapps/manifest.yaml"})
	if len(mixedNoMatch) != 0 {
		t.Error("mixedNoMatch failed")
	}

	mixedMatch1 := FilterApplicationsByPath(a1, []string{"shared/application-testing.yaml", "something/else.yaml", "more/notapps/manifest.yaml"})
	if len(mixedMatch1) != 1 {
		t.Error("mixedMatch1 failed")
	}

	mixedMatch2 := FilterApplicationsByPath(a1, []string{"somepath/application-testing.yaml", "apps/manifest.yaml", "more/notapps/manifest.yaml"})
	if len(mixedMatch2) != 1 {
		t.Error("mixedMatch2 failed")
	}

	mixedMatch3 := FilterApplicationsByPath(a1, []string{"somepath/application-testing.yaml", "something/else/dot.yaml", "more/apps/manifest.yaml"})
	if len(mixedMatch3) != 1 {
		t.Error("mixedMatch3 failed")
	}
}
