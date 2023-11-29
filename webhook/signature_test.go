package webhook

import (
	"testing"
)

const testSecret = "xyERNLDSGojWgjZls-ypjjgY$cOD"
const pingSig = "75936889306221245316c29b1207a05b915747a56bd4b906ead58e4a5de8586f"
const pingPayload = "{\"zen\":\"Half measures are as bad as nothing at all.\",\"hook_id\":441323289,\"hook\":{\"type\":\"Organization\",\"id\":441323289,\"name\":\"web\",\"active\":true,\"events\":[\"pull_request\",\"pull_request_review_comment\",\"push\"],\"config\":{\"content_type\":\"json\",\"insecure_ssl\":\"0\",\"secret\":\"********\",\"url\":\"https://argocd.k3s.vince-riv.io/webhook\"},\"updated_at\":\"2023-11-03T14:47:46Z\",\"created_at\":\"2023-11-03T14:47:46Z\",\"url\":\"https://api.github.com/orgs/vince-riv/hooks/441323289\",\"ping_url\":\"https://api.github.com/orgs/vince-riv/hooks/441323289/pings\",\"deliveries_url\":\"https://api.github.com/orgs/vince-riv/hooks/441323289/deliveries\"},\"organization\":{\"login\":\"vince-riv\",\"id\":133395678,\"node_id\":\"O_kgDOB_N03g\",\"url\":\"https://api.github.com/orgs/vince-riv\",\"repos_url\":\"https://api.github.com/orgs/vince-riv/repos\",\"events_url\":\"https://api.github.com/orgs/vince-riv/events\",\"hooks_url\":\"https://api.github.com/orgs/vince-riv/hooks\",\"issues_url\":\"https://api.github.com/orgs/vince-riv/issues\",\"members_url\":\"https://api.github.com/orgs/vince-riv/members{/member}\",\"public_members_url\":\"https://api.github.com/orgs/vince-riv/public_members{/member}\",\"avatar_url\":\"https://avatars.githubusercontent.com/u/133395678?v=4\",\"description\":\"\"},\"sender\":{\"login\":\"vrivellino\",\"id\":1489368,\"node_id\":\"MDQ6VXNlcjE0ODkzNjg=\",\"avatar_url\":\"https://avatars.githubusercontent.com/u/1489368?v=4\",\"gravatar_id\":\"\",\"url\":\"https://api.github.com/users/vrivellino\",\"html_url\":\"https://github.com/vrivellino\",\"followers_url\":\"https://api.github.com/users/vrivellino/followers\",\"following_url\":\"https://api.github.com/users/vrivellino/following{/other_user}\",\"gists_url\":\"https://api.github.com/users/vrivellino/gists{/gist_id}\",\"starred_url\":\"https://api.github.com/users/vrivellino/starred{/owner}{/repo}\",\"subscriptions_url\":\"https://api.github.com/users/vrivellino/subscriptions\",\"organizations_url\":\"https://api.github.com/users/vrivellino/orgs\",\"repos_url\":\"https://api.github.com/users/vrivellino/repos\",\"events_url\":\"https://api.github.com/users/vrivellino/events{/privacy}\",\"received_events_url\":\"https://api.github.com/users/vrivellino/received_events\",\"type\":\"User\",\"site_admin\":false}}"

func TestVerifySignatureBadLength(t *testing.T) {
	// Known payload and a secret for testing.
	knownPayload := []byte(pingPayload)
	knownSecret := testSecret
	incorrectSignature := "sha256=fffffffffffffffffffffffff"

	// Negative test case: expect false
	if VerifySignature(knownPayload, incorrectSignature, knownSecret) {
		t.Errorf("VerifySignature verified an invalid signature that's too short")
	}
}

func TestVerifySignatureBadPrefix(t *testing.T) {
	// Known payload and a secret for testing.
	knownPayload := []byte(pingPayload)
	knownSecret := testSecret
	incorrectSignature := "sha123=ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	// Negative test case: expect false
	if VerifySignature(knownPayload, incorrectSignature, knownSecret) {
		t.Errorf("VerifySignature verified a signature with an invalid prefix")
	}
}

// TestVerifySignature tests the VerifySignature function for valid and invalid scenarios.
func TestVerifySignature(t *testing.T) {
	// Known payload and a secret for testing.
	knownPayload := []byte(pingPayload)
	knownSecret := testSecret

	// Known good signature generated with the knownSecret for the knownPayload.
	// You should replace the placeholder below with an actual HMAC SHA-256 hash
	// of the `knownPayload` using `knownSecret`.
	knownGoodSignature := "sha256=" + pingSig

	// Incorrect signature for negative test case.
	incorrectSignature := "sha256=ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	// Positive test case: expect true
	if !VerifySignature(knownPayload, knownGoodSignature, knownSecret) {
		t.Errorf("VerifySignature failed to verify a correct signature")
	}

	// Negative test case: expect false
	if VerifySignature(knownPayload, incorrectSignature, knownSecret) {
		t.Errorf("VerifySignature verified an incorrect signature")
	}
}
