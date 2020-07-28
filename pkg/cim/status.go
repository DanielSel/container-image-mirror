package cim

import (
	"fmt"
	"sync/atomic"
)

func newRunnerStatus() *runnerStatus {
	return &runnerStatus{
		SourceDir: &status{
			State: make(map[string]*state),
			Count: &counter{
				Total:    &counterValue{},
				Resolved: &counterValue{},
				Copied:   &counterValue{},
				Deleted:  &counterValue{},
				Failed:   &counterValue{},
			},
		},
		Image: &status{
			State: make(map[string]*state),
			Count: &counter{
				Total:    &counterValue{},
				Resolved: &counterValue{},
				Copied:   &counterValue{},
				Deleted:  &counterValue{},
				Failed:   &counterValue{},
			},
		},
		Tag: &status{
			State: make(map[string]*state),
			Count: &counter{
				Total:    &counterValue{},
				Resolved: &counterValue{},
				Copied:   &counterValue{},
				Deleted:  &counterValue{},
				Failed:   &counterValue{},
			},
		},
	}
}

type runnerStatus struct {
	SourceDir *status
	Image     *status
	Tag       *status
}

// Count produces a summary of counter values based on the given filters (as arguments)
// Each filter that is set to '' is treated as wildcard for all types
func (r *runnerStatus) Count(objType ObjectType, counterType CounterType) uint64 {
	var result uint64
	var counterSet []*counter

	switch {
	case objType == "":
		fallthrough
	case objType == "*":
		counterSet = []*counter{r.SourceDir.Count, r.Image.Count, r.Tag.Count}
	case objType == ObjectTypeSourceDir:
		counterSet = []*counter{r.SourceDir.Count}
	case objType == ObjectTypeImage:
		counterSet = []*counter{r.Image.Count}
	case objType == ObjectTypeTag:
		counterSet = []*counter{r.Tag.Count}
	}

	switch {
	case counterType == "":
		for _, counter := range counterSet {
			result += counter.Total.val + counter.Resolved.val + counter.Copied.val + counter.Failed.val
		}
	case counterType == CounterTypeTotal:
		for _, counter := range counterSet {
			result += counter.Total.val
		}
	case counterType == CounterTypeResolved:
		for _, counter := range counterSet {
			result += counter.Resolved.val
		}
	case counterType == CounterTypeCopied:
		for _, counter := range counterSet {
			result += counter.Copied.val
		}
	case counterType == CounterTypeDeleted:
		for _, counter := range counterSet {
			result += counter.Copied.val
		}
	case counterType == CounterTypeFailed:
		for _, counter := range counterSet {
			result += counter.Failed.val
		}
	}
	return result
}

func (r *runnerStatus) StatusByObjType(ty ObjectType) (*status, error) {
	switch ty {
	case ObjectTypeSourceDir:
		return r.SourceDir, nil
	case ObjectTypeImage:
		return r.Image, nil
	case ObjectTypeTag:
		return r.Tag, nil
	default:
		return nil, fmt.Errorf("Unknown ObjectType: %s", ty)
	}
}

type status struct {
	Count *counter
	State map[string]*state
}

func (s *status) SetTotal(total uint64) {
	s.Count.Total.val = total
}

func (s *status) Set(key string, ty StatusType, val bool) error {
	switch ty {
	case StatusTypeResolved:
		s.ensureKey(key)
		s.State[key].Resolved = val
		if val {
			s.Count.Resolved.Inc()
		} else {
			s.Count.Failed.Inc()
		}
		return nil
	case StatusTypeCopied:
		s.ensureKey(key)
		s.State[key].Copied = val
		if val {
			s.Count.Copied.Inc()
		} else {
			s.Count.Failed.Inc()
		}
		return nil
	case StatusTypeDeleted:
		s.ensureKey(key)
		s.State[key].Deleted = val
		if val {
			s.Count.Deleted.Inc()
		} else {
			s.Count.Failed.Inc()
		}
		return nil
	default:
		return fmt.Errorf("Unknown StatusType: %s", ty)
	}
}

func (s *status) ensureKey(key string) {
	if _, ok := s.State[key]; !ok {
		s.State[key] = &state{}
	}
}

type state struct {
	Resolved, Copied, Deleted bool
}

type counter struct {
	Total, Resolved, Copied, Deleted, Failed *counterValue
}

type counterValue struct {
	val uint64
}

func (c *counterValue) Inc() {
	atomic.AddUint64(&c.val, 1)
}

type CounterType string

const (
	CounterTypeTotal    CounterType = "Total"
	CounterTypeResolved CounterType = "Resolved"
	CounterTypeCopied   CounterType = "Copied"
	CounterTypeDeleted  CounterType = "Deleted"
	CounterTypeFailed   CounterType = "Failed"
)
