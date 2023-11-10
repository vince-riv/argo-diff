package argocd

import (
	"testing"
)


func TestFilterApplications(t *testing.T) {
	var a []Application

	result, _ := filterApplications(a, "o", "r", "m", "m")
	if len(result) != 0 { t.Error("Empty param didn't lead to empty result")}
	payload, _, err := readFileToByteArray(payloadAppList)
	if err != nil { t.Errorf("Failed to read %s: %v", payloadAppList, err) }
	a, err = decodeApplicationListPayload(payload)
	if err != nil { t.Errorf("decodeApplicationListPayload() failed: %s", err) }

	result, _ = filterApplications(a, "o", "r", "m", "m")
	if len(result) != 0 { t.Error("Unmatchable params didn't lead to empty result")}

	result, _ = filterApplications(a, "vince-riv", "argo-diff", "main", "main")
	if len(result) != 0 { t.Error("Push to main shouldn't have matched")}

	result, _ = filterApplications(a, "vince-riv", "argo-diff", "main", "dev")
	if len(result) != 1 { t.Error("Push to dev should have matched")}

	a[0].Spec.Source.TargetRevision = "main"
	a[1].Spec.Source.TargetRevision = "main"
	result, _ = filterApplications(a, "vince-riv", "argo-diff", "main", "main")
	if len(result) != 0 { t.Error("Push to main shouldn't have matched (targetRev main)")}

	result, _ = filterApplications(a, "vince-riv", "argo-diff", "main", "dev")
	if len(result) != 1 { t.Error("Push to dev should have matched (targetRev main)")}
}
