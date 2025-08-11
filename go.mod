module github.com/vince-riv/argo-diff

go 1.23.0

toolchain go1.23.9

require (
	github.com/bradleyfalzon/ghinstallation/v2 v2.16.0
	github.com/google/go-github/v72 v72.0.0
	github.com/hexops/gotextdiff v1.0.3
	github.com/rs/zerolog v1.34.0
	github.com/spf13/pflag v1.0.6
	k8s.io/apimachinery v0.31.0
	sigs.k8s.io/yaml v1.4.0
)

require (
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/utils v0.0.0-20240711033017-18e509b52bc8 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.4-0.20241211184406-7bf59b3d70ee // indirect
)

replace (
	// Pulled from https://github.com/argoproj/argo-cd/blob/v2.13.2/go.mod
	github.com/go-telegram-bot-api/telegram-bot-api/v5 => github.com/OvyFlash/telegram-bot-api/v5 v5.0.0-20240108230938-63e5c59035bf

	github.com/golang/protobuf => github.com/golang/protobuf v1.5.4
	github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.16.0

	// Avoid CVE-2022-3064
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.4.0

	// Avoid CVE-2022-28948
	gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.1

	k8s.io/api => k8s.io/api v0.31.0
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.31.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.31.0
	k8s.io/apiserver => k8s.io/apiserver v0.31.0
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.31.0
	k8s.io/client-go => k8s.io/client-go v0.31.0
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.31.0
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.31.0
	k8s.io/code-generator => k8s.io/code-generator v0.31.0
	k8s.io/component-base => k8s.io/component-base v0.31.0
	k8s.io/component-helpers => k8s.io/component-helpers v0.31.0
	k8s.io/controller-manager => k8s.io/controller-manager v0.31.0
	k8s.io/cri-api => k8s.io/cri-api v0.31.0
	k8s.io/cri-client => k8s.io/cri-client v0.31.0
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.31.0
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.31.0
	k8s.io/endpointslice => k8s.io/endpointslice v0.31.0
	k8s.io/kms => k8s.io/kms v0.31.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.31.0
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.31.0
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.31.0
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.31.0
	k8s.io/kubectl => k8s.io/kubectl v0.31.0
	k8s.io/kubelet => k8s.io/kubelet v0.31.0
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.31.0
	k8s.io/metrics => k8s.io/metrics v0.31.0
	k8s.io/mount-utils => k8s.io/mount-utils v0.31.0
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.31.0
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.31.0
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.31.0
	k8s.io/sample-controller => k8s.io/sample-controller v0.31.0
)
