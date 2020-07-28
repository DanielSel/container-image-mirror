package cim

import (
	"fmt"
	"sync"
	"testing"

	"github.com/danielsel/container-image-mirror/pkg/cim/registry"
	"github.com/stretchr/testify/assert"
)

func Benchmark_InterleaveBuffer(b *testing.B) {
	// Parameters
	numInterval := []uint{4, 32, 128, 512, 1024}
	numImages := []uint32{1, 8, 16, 32, 256, 1024}
	numTagsPerImage := []uint32{1, 8, 16, 32, 256, 1024}

	// Benchmark Loop
	for _, c := range numInterval {
		for _, i := range numImages {
			for _, t := range numTagsPerImage {
				// Only run if total num of tags is within global limit
				if uint(i)*uint(t) > ConfigGlobalTagLimit {
					continue
				}
				b.Run(fmt.Sprintf("%d_%dx%d", c, i, t), func(b *testing.B) {
					testdata := genMultiImageCopyJobs(i, t)
					for n := 0; n < b.N; n++ {
						_ = testInterleaveBuffer(c, testdata)
					}
				})
			}
		}
	}
}

func Test_InterleaveBuffer(t *testing.T) {
	// Arrange
	var testdata []*copyJob
	testdata = append(testdata, genImageCopyJobs("test.registry.url/test/image/a", 5)...)
	testdata = append(testdata, genImageCopyJobs("test.registry.url/test/image/b", 1)...)
	testdata = append(testdata, genImageCopyJobs("test.registry.url/test/image/c", 2)...)
	testdata = append(testdata, genImageCopyJobs("test.registry.url/test/image/d", 6)...)
	expected := []*copyJob{
		copyJobDst("test.registry.url/test/image/a:tag0"),
		copyJobDst("test.registry.url/test/image/b:tag0"),
		copyJobDst("test.registry.url/test/image/c:tag0"),
		copyJobDst("test.registry.url/test/image/d:tag0"),
		copyJobDst("test.registry.url/test/image/a:tag1"),
		copyJobDst("test.registry.url/test/image/c:tag1"),
		copyJobDst("test.registry.url/test/image/d:tag1"),
		copyJobDst("test.registry.url/test/image/a:tag2"),
		copyJobDst("test.registry.url/test/image/d:tag2"),
		copyJobDst("test.registry.url/test/image/a:tag3"),
		copyJobDst("test.registry.url/test/image/d:tag3"),
		copyJobDst("test.registry.url/test/image/a:tag4"),
		copyJobDst("test.registry.url/test/image/d:tag4"),
		copyJobDst("test.registry.url/test/image/d:tag5"),
	}

	// Act
	actual := testInterleaveBuffer(4, testdata)

	// Assert
	assert.ElementsMatch(t, expected, actual)
}

func testInterleaveBuffer(capacity uint, testdata []*copyJob) []*copyJob {
	var results []*copyJob
	ib := NewInterleaveBuffer(capacity)

	// Helper for flushing buffer
	tmpch := make(chan copyJob)
	tmpwg := sync.WaitGroup{}
	tmpwg.Add(1)
	go func() {
		for x := range tmpch {
			job := x
			results = append(results, &job)
		}
		tmpwg.Done()
	}()

	for _, x := range testdata {
		job := ib.Tick(x)
		if job != nil {
			results = append(results, job)
		}
	}
	ib.FlushBuffer(tmpch)
	close(tmpch)
	tmpwg.Wait()

	return results
}

func genMultiImageCopyJobs(numImages, numTags uint32) []*copyJob {
	var jobs []*copyJob
	count := int(numImages)
	for i := 0; i < count; i++ {
		jobs = append(genImageCopyJobs(fmt.Sprintf("test.registry.url/test/image%d", i), numTags))
	}
	return jobs
}

func genImageCopyJobs(image string, numTags uint32) []*copyJob {
	var jobs []*copyJob
	count := int(numTags)
	for i := 0; i < count; i++ {
		jobs = append(jobs, copyJobDst(fmt.Sprintf("%s:tag%d", image, i)))
	}
	return jobs
}

func copyJobDst(dst string) *copyJob {
	return &copyJob{dst: &registry.TagDetails{Original: dst}}
}
