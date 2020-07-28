package cim

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cimapi "github.com/danielsel/container-image-mirror/api"
	"github.com/danielsel/container-image-mirror/pkg/cim"
	"github.com/danielsel/container-image-mirror/pkg/testlogr"
)

func Test_CIM_DockerHub_To_Single_GCR(t *testing.T) {
	// Testdata
	testConfig := &cimapi.MirrorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-mirror-cfg",
		},
		Spec: cimapi.MirrorConfigSpec{
			Images: []string{
				"docker.io/istio",
			},
			Destinations: []string{
				"gcr.io/sel-red-alpha/mirror",
			},
			TagPolicy: &cimapi.TagPolicy{
				MinNum: 3,
				MaxNum: 12,
				MaxAge: metav1.Duration{Duration: 3 * 27 * 24 * time.Hour}, // 3x 27d
			},
		},
	}

	// Arrange
	log := testlogr.NewTestLogger(t)
	log.(*testlogr.TestLogger).SetLogLevel(0)
	runner := cim.NewRunner(testConfig, log)

	// Act
	err := runner.Run(context.Background())

	// Assert
	if err != nil {
		t.Fatal(err)
	}
}
