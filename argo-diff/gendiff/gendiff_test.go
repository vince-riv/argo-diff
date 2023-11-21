package gendiff

import (
	"testing"
)

var (
	curManifests []string
	newManifests []string
)

func init() {
	curManifests = append(curManifests, "{\"apiVersion\":\"v1\",\"kind\":\"Service\",\"metadata\":{\"labels\":{\"app\":\"argo-diff\",\"app.kubernetes.io/component\":\"webhook-processor\",\"app.kubernetes.io/instance\":\"argo-diff\",\"app.kubernetes.io/name\":\"argo-diff\",\"argocd.argoproj.io/instance\":\"argo-diff\"},\"name\":\"argo-diff\",\"namespace\":\"argocd\"},\"spec\":{\"ports\":[{\"name\":\"http\",\"port\":8080,\"protocol\":\"TCP\",\"targetPort\":8080}],\"selector\":{\"app.kubernetes.io/component\":\"webhook-processor\",\"app.kubernetes.io/instance\":\"argo-diff\",\"app.kubernetes.io/name\":\"argo-diff\"},\"type\":\"ClusterIP\"}}")
	curManifests = append(curManifests, "{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"labels\":{\"app\":\"argo-diff\",\"app.kubernetes.io/component\":\"webhook-processor\",\"app.kubernetes.io/instance\":\"argo-diff\",\"app.kubernetes.io/name\":\"argo-diff\",\"argocd.argoproj.io/instance\":\"argo-diff\"},\"name\":\"argo-diff\",\"namespace\":\"argocd\"},\"spec\":{\"replicas\":1,\"selector\":{\"matchLabels\":{\"app\":\"argo-diff\"}},\"template\":{\"metadata\":{\"labels\":{\"app\":\"argo-diff\",\"app.kubernetes.io/component\":\"webhook-processor\",\"app.kubernetes.io/instance\":\"argo-diff\",\"app.kubernetes.io/name\":\"argo-diff\"}},\"spec\":{\"containers\":[{\"envFrom\":[{\"secretRef\":{\"name\":\"argo-diff-env\"}}],\"image\":\"529264564500.dkr.ecr.us-east-1.amazonaws.com/argo-diff:main\",\"imagePullPolicy\":\"Always\",\"livenessProbe\":{\"httpGet\":{\"path\":\"/healthz\",\"port\":\"http\"},\"initialDelaySeconds\":2,\"periodSeconds\":10},\"name\":\"worker\",\"ports\":[{\"containerPort\":8080,\"name\":\"http\",\"protocol\":\"TCP\"}],\"readinessProbe\":{\"httpGet\":{\"path\":\"/healthz\",\"port\":\"http\"},\"initialDelaySeconds\":2,\"periodSeconds\":10},\"resources\":{},\"startupProbe\":{\"failureThreshold\":10,\"httpGet\":{\"path\":\"/healthz\",\"port\":\"http\"},\"periodSeconds\":2}}],\"imagePullSecrets\":[{\"name\":\"ecr-login\"}]}}}}")
	curManifests = append(curManifests, "{\"apiVersion\":\"bitnami.com/v1alpha1\",\"kind\":\"SealedSecret\",\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"argocd.argoproj.io/instance\":\"argo-diff\"},\"name\":\"argo-diff-env\",\"namespace\":\"argocd\"},\"spec\":{\"encryptedData\":{\"GITHUB_API_TOKEN\":\"AgCfXwlshq0xbwRfP6v6rM6Sx1oRja3Ueh+wOs0D0w/n6+jnT2sLiEs7bQh5cntNc/3Ze+BFhPgsydFhAxeU3Pe3hcBkECuaJIP2eTBfqbCDd6SKrA3l4/jTRxHK/HexJH0+nk3ohkmmg+FX0bh/sckzXjwGTdrKaHut8+sawesqdbmWteQZMDjWETKa/viAIEQervwaE8Pg83UoSkmKPACNY9buEXaJ4scyIQx3ckNt8yBIaUBV0FRX5sE+AEpgtfKudQ+XUU59PZllAAKqNGaPEac1zjKtIj0ar44GSuCxW/r/AEEOogSe5ujoUfIW2A7lt7NSSYrinratqk943PylqeI7J8038x37WwYYoyJhfsq8eOh1y7JT2bEj940XX755V/x2faL+yryhUOMD9+dPlyuuuNxiot5pOgV/3T6x6hIQz9AvOsIm5PDFbCDLwsmrZrbVoln58+xq42rFvJs0nXXVaZMxNuQSQyAWe46VxajTYS6HXLNKkuz4t5MW0DYvopLvzd8zq4oi1aZGxdOiA2AYj+iSScrXsupuo/QlE2rie4xmAMpczicp/fr6ZUd8YvOsvbLCnLjRMGZgCRZKYdvyU6Bb3kICcfufN7YIfhLQA2SCAYWS+mYePRKoV4zyEYxAVnkKXB6cBAZv3yBpaZDg0f11POqJNHlhW8J6d/ZckS0qEN8lgAzkSymzngBNC8CyQEXpv1Pg\",\"GITHUB_WEBHOOK_SECRET\":\"AgASVAMhHzY5eQBdVDAGea4qG2JZi3Ofpbx+Cv2Fot9ZxoExIYsWXiJOOa2mJJk1r30PT7AsvNCwJWuPVOsepXF4qRwdY7knagEtWD8VqtHOClzkrZSg9ib5CcKo7ln6iQGbVZz9RGZl/Hl6YC8m+ddo+V0Kin0j35EhnFD6hO5fosLCLgPH8VRBKLxpahzlqB6W5gj0rDFDC+8HpAnnh1ApRQ9Td019O3Bvii+cXPCwhdpXHunytNogVqgOllbZBQ0xZ8RdZXPttaXlV/Lc8GnlcyU/Pmbkta9YvJLXLjXaIDd0rdvPOAAH34YzPpd0l0oRUAF/0UWeyLjaofMHzST+OSpKNzEXeqBei1GB+AXkDsbQLwxUcFYVjAlRpOc612tRyJvGMVSKtfeQFzK0Qr0E4ai5BYMB/R4u3M0m1lT0RkVpfnDF8JcA2s2ksAcNK9E2kXAIMGPdziegCFr4G4kkBI5tSKco4EsYnOtWtEkh6Q2uWDyWRvauvy8R1lvX1unb5z6SjYLdJSBkNZQHURfPLNM0K9bCgbzhmrNBc/7Sjkci/ptllRWRmfkCMzZKCusuVjtxIIxXIgXxhDkdRZrcT68d4HHTpsfHMCVWhxO7ZtH/owE2C43Lw38ooX4voFbB0/G5qwv7RWSSq6X9pd04sqIVj6I0KngQkUd3//KwtXiERiBrMAUvC4dJmJnBqB8PGW1kY+ifDJuwqyjjhGn4ya5Ddu8ZsWSTsKwW\"},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"name\":\"argo-diff-env\",\"namespace\":\"argocd\"}}}}")
	curManifests = append(curManifests, "{\"apiVersion\":\"traefik.containo.us/v1alpha1\",\"kind\":\"IngressRoute\",\"metadata\":{\"labels\":{\"argocd.argoproj.io/instance\":\"argo-diff\"},\"name\":\"argocd-diff\",\"namespace\":\"argocd\"},\"spec\":{\"entryPoints\":[\"websecure\"],\"routes\":[{\"kind\":\"Rule\",\"match\":\"Host(`argocd.k3s.vince-riv.io`) \\u0026\\u0026 HeadersRegexp(`X-GitHub-Event`, `.*`) \\u0026\\u0026 PathPrefix(`/webhook`)\",\"priority\":100,\"services\":[{\"name\":\"argo-diff\",\"port\":8080}]}],\"tls\":{\"certResolver\":\"default\"}}}")

	newManifests = append(newManifests, "{\"apiVersion\":\"v1\",\"kind\":\"Service\",\"metadata\":{\"labels\":{\"app\":\"argo-diff\",\"app.kubernetes.io/component\":\"webhook-processor\",\"app.kubernetes.io/instance\":\"argo-diff\",\"app.kubernetes.io/name\":\"argo-diff\",\"argocd.argoproj.io/instance\":\"argo-diff\"},\"name\":\"argo-diff\",\"namespace\":\"argocd\"},\"spec\":{\"ports\":[{\"name\":\"http\",\"port\":8080,\"protocol\":\"TCP\",\"targetPort\":8080}],\"selector\":{\"app.kubernetes.io/component\":\"webhook-processor\",\"app.kubernetes.io/instance\":\"argo-diff\",\"app.kubernetes.io/name\":\"argo-diff\"},\"type\":\"ClusterIP\"}}")
	newManifests = append(newManifests, "{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"labels\":{\"app\":\"argo-diff\",\"app.kubernetes.io/component\":\"webhook-processor\",\"app.kubernetes.io/instance\":\"argo-diff\",\"app.kubernetes.io/name\":\"argo-diff\",\"argocd.argoproj.io/instance\":\"argo-diff\"},\"name\":\"argo-diff\",\"namespace\":\"argocd\"},\"spec\":{\"replicas\":2,\"selector\":{\"matchLabels\":{\"app\":\"argo-diff\"}},\"template\":{\"metadata\":{\"labels\":{\"app\":\"argo-diff\",\"app.kubernetes.io/component\":\"webhook-processor\",\"app.kubernetes.io/instance\":\"argo-diff\",\"app.kubernetes.io/name\":\"argo-diff\"}},\"spec\":{\"containers\":[{\"envFrom\":[{\"secretRef\":{\"name\":\"argo-diff-env\"}}],\"image\":\"529264564500.dkr.ecr.us-east-1.amazonaws.com/argo-diff:main\",\"imagePullPolicy\":\"Always\",\"livenessProbe\":{\"httpGet\":{\"path\":\"/healthz\",\"port\":\"http\"},\"initialDelaySeconds\":2,\"periodSeconds\":10},\"name\":\"worker\",\"ports\":[{\"containerPort\":8080,\"name\":\"http\",\"protocol\":\"TCP\"}],\"readinessProbe\":{\"httpGet\":{\"path\":\"/healthz\",\"port\":\"http\"},\"initialDelaySeconds\":2,\"periodSeconds\":10},\"resources\":{},\"startupProbe\":{\"failureThreshold\":10,\"httpGet\":{\"path\":\"/healthz\",\"port\":\"http\"},\"periodSeconds\":2}}],\"imagePullSecrets\":[{\"name\":\"ecr-login\"}]}}}}")
	newManifests = append(newManifests, "{\"apiVersion\":\"bitnami.com/v1alpha1\",\"kind\":\"SealedSecret\",\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"argocd.argoproj.io/instance\":\"argo-diff\"},\"name\":\"argo-diff-env\",\"namespace\":\"argocd\"},\"spec\":{\"encryptedData\":{\"GITHUB_API_TOKEN\":\"AgCfXwlshq0xbwRfP6v6rM6Sx1oRja3Ueh+wOs0D0w/n6+jnT2sLiEs7bQh5cntNc/3Ze+BFhPgsydFhAxeU3Pe3hcBkECuaJIP2eTBfqbCDd6SKrA3l4/jTRxHK/HexJH0+nk3ohkmmg+FX0bh/sckzXjwGTdrKaHut8+sawesqdbmWteQZMDjWETKa/viAIEQervwaE8Pg83UoSkmKPACNY9buEXaJ4scyIQx3ckNt8yBIaUBV0FRX5sE+AEpgtfKudQ+XUU59PZllAAKqNGaPEac1zjKtIj0ar44GSuCxW/r/AEEOogSe5ujoUfIW2A7lt7NSSYrinratqk943PylqeI7J8038x37WwYYoyJhfsq8eOh1y7JT2bEj940XX755V/x2faL+yryhUOMD9+dPlyuuuNxiot5pOgV/3T6x6hIQz9AvOsIm5PDFbCDLwsmrZrbVoln58+xq42rFvJs0nXXVaZMxNuQSQyAWe46VxajTYS6HXLNKkuz4t5MW0DYvopLvzd8zq4oi1aZGxdOiA2AYj+iSScrXsupuo/QlE2rie4xmAMpczicp/fr6ZUd8YvOsvbLCnLjRMGZgCRZKYdvyU6Bb3kICcfufN7YIfhLQA2SCAYWS+mYePRKoV4zyEYxAVnkKXB6cBAZv3yBpaZDg0f11POqJNHlhW8J6d/ZckS0qEN8lgAzkSymzngBNC8CyQEXpv1Pg\",\"GITHUB_WEBHOOK_SECRET\":\"AgASVAMhHzY5eQBdVDAGea4qG2JZi3Ofpbx+Cv2Fot9ZxoExIYsWXiJOOa2mJJk1r30PT7AsvNCwJWuPVOsepXF4qRwdY7knagEtWD8VqtHOClzkrZSg9ib5CcKo7ln6iQGbVZz9RGZl/Hl6YC8m+ddo+V0Kin0j35EhnFD6hO5fosLCLgPH8VRBKLxpahzlqB6W5gj0rDFDC+8HpAnnh1ApRQ9Td019O3Bvii+cXPCwhdpXHunytNogVqgOllbZBQ0xZ8RdZXPttaXlV/Lc8GnlcyU/Pmbkta9YvJLXLjXaIDd0rdvPOAAH34YzPpd0l0oRUAF/0UWeyLjaofMHzST+OSpKNzEXeqBei1GB+AXkDsbQLwxUcFYVjAlRpOc612tRyJvGMVSKtfeQFzK0Qr0E4ai5BYMB/R4u3M0m1lT0RkVpfnDF8JcA2s2ksAcNK9E2kXAIMGPdziegCFr4G4kkBI5tSKco4EsYnOtWtEkh6Q2uWDyWRvauvy8R1lvX1unb5z6SjYLdJSBkNZQHURfPLNM0K9bCgbzhmrNBc/7Sjkci/ptllRWRmfkCMzZKCusuVjtxIIxXIgXxhDkdRZrcT68d4HHTpsfHMCVWhxO7ZtH/owE2C43Lw38ooX4voFbB0/G5qwv7RWSSq6X9pd04sqIVj6I0KngQkUd3//KwtXiERiBrMAUvC4dJmJnBqB8PGW1kY+ifDJuwqyjjhGn4ya5Ddu8ZsWSTsKwW\"},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"name\":\"argo-diff-env\",\"namespace\":\"argocd\"}}}}")
	newManifests = append(newManifests, "{\"apiVersion\":\"traefik.containo.us/v1alpha1\",\"kind\":\"IngressRoute\",\"metadata\":{\"labels\":{\"argocd.argoproj.io/instance\":\"argo-diff\"},\"name\":\"argocd-diff\",\"namespace\":\"argocd\"},\"spec\":{\"entryPoints\":[\"websecure\"],\"routes\":[{\"kind\":\"Rule\",\"match\":\"Host(`argo-diff.k3s.vince-riv.io`) \\u0026\\u0026 HeadersRegexp(`X-GitHub-Event`, `.*`) \\u0026\\u0026 PathPrefix(`/webhook`)\",\"priority\":100,\"services\":[{\"name\":\"argo-diff\",\"port\":8080}]}],\"tls\":{\"certResolver\":\"default\"}}}")
}

func TestK8sJsonToYaml(t *testing.T) {
	testStr := "{\"key\": \"val1\"}"
	_, err := k8sJsonToYaml(testStr)
	if err == nil {
		t.Error("k8sJsonToYaml() should have return an error")
	}
	testStr = "{\"apiVersion\": \"v1\", \"kind\": \"Service\", \"metadata\": {\"name\": \"svc\"}}"
	r, err := k8sJsonToYaml(testStr)
	if err != nil {
		t.Error("k8sJsonToYaml() shouldn't have return an error")
	}
	if r.YamlStr != "apiVersion: v1\nkind: Service\nmetadata:\n  name: svc\n" {
		t.Error("k8sJsonToYaml() returned unexpected YAML: " + r.YamlStr)
	}
	if r.Filename != "v1_Service_svc" {
		t.Error("k8sJsonToYaml() returned unexpected filename: " + r.Filename)
	}
}

func TestUnifiedDiff(t *testing.T) {
	str1 := "key: val1\n"
	str2 := "key: val2\n"
	diff := unifiedDiff("test1.yaml", "test2.yaml", str1, str2)
	expectedDiff := "--- test1.yaml\n+++ test2.yaml\n@@ -1 +1 @@\n-key: val1\n+key: val2\n"
	// println(diff)
	if diff == "" {
		t.Error("unifiedDiff() produced empty diff")
	}
	if diff != expectedDiff {
		t.Error("unifiedDiff() produced unexpected diff")
	}
}

func TestUnifiedDiffNoDiff(t *testing.T) {
	str1 := "key: val1\n"
	str2 := "key: val1\n"
	diff := unifiedDiff("test1.yaml", "test2.yaml", str1, str2)
	if diff != "" {
		t.Error("unifiedDiff() didn't produce an empty diff")
	}
}

func TestK8sAppDiff(t *testing.T) {
	diffStr, err := K8sAppDiff(curManifests, newManifests)
	//println(diffStr)
	if err != nil {
		t.Error("Unexpected error: " + err.Error())
	}
	if diffStr == "" {
		t.Error("Empty diff produced")
	}
}
