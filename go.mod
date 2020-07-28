module github.com/danielsel/container-image-mirror

go 1.13

require (
	github.com/cloudfoundry/bytefmt v0.0.0-20200131002437-cf55d5288a48
	github.com/elastic/go-sysinfo v1.4.0
	github.com/go-logr/logr v0.2.1-0.20200730175230-ee2de8da5be6
	github.com/google/go-cmp v0.5.2 // indirect
	github.com/google/go-containerregistry v0.1.3-0.20200901174039-071a121b9eee
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/maxbrunsfeld/counterfeiter/v6 v6.2.3 // indirect
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/profile v1.5.0
	github.com/prometheus/common v0.13.0 // indirect
	github.com/rs/xid v1.2.1
	github.com/stretchr/testify v1.5.1
	go.uber.org/zap v1.16.0
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a // indirect
	golang.org/x/sys v0.0.0-20200831180312-196b9ba8737a // indirect
	golang.org/x/tools v0.0.0-20200902151623-5b9ef244dc36 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v0.19.0
	k8s.io/code-generator v0.19.0 // indirect
	k8s.io/gengo v0.0.0-20200728071708-7794989d0000 // indirect
	k8s.io/klog/v2 v2.3.0 // indirect
	k8s.io/kube-openapi v0.0.0-20200831175022-64514a1d5d59 // indirect
	k8s.io/utils v0.0.0-20200821003339-5e75c0163111 // indirect
	sigs.k8s.io/controller-runtime v0.6.1-0.20200902144306-f2d4ad78c7ab
	sigs.k8s.io/structured-merge-diff/v3 v3.0.0 // indirect
	sigs.k8s.io/yaml v1.2.0
)

replace github.com/google/go-containerregistry => github.com/danielsel/go-containerregistry v0.1.3-0.20200914211052-c13355b39aa5
