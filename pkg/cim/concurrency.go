package cim

import (
	"sync"

	"github.com/danielsel/container-image-mirror/pkg/cim/registry"
)

func splitJobCh(in chan copyJob) (chan copyJob, chan copyJob) {
	out1 := make(chan copyJob, ConfigBufferSize)
	out2 := make(chan copyJob, ConfigBufferSize)
	go func() {
		for x := range in {
			val := x
			out1 <- val
			out2 <- val
		}
		close(out1)
		close(out2)
	}()
	return out1, out2
}

func mergeStringChs(in ...chan string) chan string {
	// Use waitgroup to wait for all results and merge
	var wg sync.WaitGroup
	out := make(chan string)
	forwarder := func(c <-chan string) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(in))
	for _, c := range in {
		go forwarder(c)
	}

	// Start a goroutine to close out once all the output goroutines are done.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func mergeTagDetailChs(in ...chan *registry.TagDetails) chan *registry.TagDetails {
	// Use waitgroup to wait for all results and merge
	var wg sync.WaitGroup
	out := make(chan *registry.TagDetails)
	forwarder := func(c <-chan *registry.TagDetails) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(in))
	for _, c := range in {
		go forwarder(c)
	}

	// Start a goroutine to close out once all the output goroutines are done.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func mergeEventChs(in ...chan Event) chan Event {
	// Use waitgroup to wait for all results and merge
	var wg sync.WaitGroup
	out := make(chan Event)
	forwarder := func(c <-chan Event) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(in))
	for _, c := range in {
		go forwarder(c)
	}

	// Start a goroutine to close out once all the output goroutines are done.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
