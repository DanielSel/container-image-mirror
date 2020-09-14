/**
	memlim provides project-global utility functions that hook into golang runtime tools
	in an attempt to provide memory management features for applications
	that can quickly consume large amounts of memory if not slowed down.

	Unfourtunately, golang's reporting on memory state is unreliable.
	memlim imlements it's own memory usage tracking to provide additional reliability
	for memory management related decision making. Therefore it is very important that
	you

	**Call memlim.LimitMemory(<maxSizeInBytes>) at the very beginning of your program execution**

	and

	**Consistently call WaitForFreeMemory with precise estimations and stable LoadSheddingFnc's**

	in order for memlim to work smoothly. Running benchmarks on your stuff ahead of using memlim is recommended.
	Overestimating in case of uncertainty is also recommended.
**/
package memlim

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/cloudfoundry/bytefmt"
	sysinfo "github.com/elastic/go-sysinfo"
	"github.com/go-logr/logr"
)

var DefaultInterval = 4 * time.Second
var Interval = DefaultInterval

var running bool
var ctx, cnFnc = context.WithCancel(context.Background())
var log logr.Logger
var queue []goRoutine
var activeGoRoutines []goRoutine
var runmx = &sync.Mutex{}
var memlimit, memtrack uint64
var freedmemhints bool

type goRoutine struct {
	memoryRequest uint64
	cancelFnc     LoadSheddingFnc
	waitCh        chan struct{}
}

type LoadSheddingFnc func()

// LimitMemory attempts to limit memory consumption by enabling WaitForFreeMemory
// to wait for sufficient available memory and manages recognized overconsumption
// by executing LoadSheddingFnc's
func LimitMemory(maxHeapSize uint64) context.CancelFunc {
	memlimit = maxHeapSize
	log.Info("Memory limit set",
		"MaxHeapSize", bytefmt.ByteSize(maxHeapSize),
	)

	// If already running, kill and restart with new params
	if running {
		cnFnc()
		ctx, cnFnc = context.WithCancel(context.Background())
		running = false
		return LimitMemory(maxHeapSize)
	}

	// Timer
	go func() {
		for {
			select {
			case <-ctx.Done():
				running = false
				return
			case <-time.After(Interval):
				runMemoryManagement()
			}
		}
	}()

	running = true
	return cnFnc
}

func LimitMemoryDynamic() error {
	host, err := sysinfo.Host()
	if err != nil {
		return fmt.Errorf("Unable to determine host information: %w", err)
	}
	mem, err := host.Memory()
	if err != nil {
		return fmt.Errorf("Unable to determine memory information from host: %w", err)
	}
	memlimit = mem.Available

	heapLimit := mem.Available / 10
	log.Info("Limiting dynamic memory allocation", "AvailableMemory", mem.Available, "MaxHeapSize", heapLimit)
	LimitMemory(heapLimit)
	return nil
}

// WaitForFreeMemory suspends the execution of the caller's thread until `RequestedMem` bytes of memory are available
// or the caller-provided `ctx` is canceled.
// The cancelFnc is expected to free up approximately RequestdedMem bytes, making the efficiency and stability
// of memlim highly dependent on precise estimations of how much memory the following operation will require.
func WaitForFreeMemory(ctx context.Context, RequestedMem uint64, cancelFnc LoadSheddingFnc) {
	if !running {
		panic("Memory management is not running - can't wait for free memory")
	}
	meCh := make(chan struct{})
	queue = append(queue, goRoutine{
		memoryRequest: RequestedMem,
		cancelFnc:     cancelFnc,
		waitCh:        meCh,
	})
	log.V(1).Info("WaitForFreeMemory request received",
		"MemoryRequested", bytefmt.ByteSize(RequestedMem))
	select {
	case <-meCh:
	case <-ctx.Done():
	}
}

func FreedMemHint(freedMemBytes uint64) {
	memtrack -= freedMemBytes
	if !freedmemhints {
		freedmemhints = true
		DefaultInterval = 200 * time.Millisecond
	}
}

func ReadMemAlloc() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func SetLogger(logger logr.Logger) {
	log = logger
}

func runMemoryManagement() {
	runmx.Lock()

	alloc := ReadMemAlloc()
	used := alloc
	if freedmemhints && memtrack > alloc {
		used = memtrack
	}
	avail := memlimit - used
	fac := float64(alloc) / float64(memlimit)
	log.V(2).Info("memlim statistics",
		"MemoryAllocated", bytefmt.ByteSize(alloc),
		"MemoryUsed", bytefmt.ByteSize(used),
		"MemoryAvailable", bytefmt.ByteSize(avail),
		"MemoryLimit", bytefmt.ByteSize(memlimit),
		"MemoryUsagePercent", fmt.Sprintf("%.2f", fac))

	switch {
	case fac > 1.1:
		// Emergency, trigger load shedding and check back shortly
		Interval = 1 * time.Second
		log.Info("WARNING: Shedding load because memory consumption crossed threshold",
			"MemoryAllocated", bytefmt.ByteSize(alloc),
			"MemoryUsed", bytefmt.ByteSize(used),
			"MemoryAvailable", bytefmt.ByteSize(avail),
			"MemoryLimit", bytefmt.ByteSize(memlimit),
			"MemoryUsagePercent", fmt.Sprintf("%.2f", fac))
		go shedLoad()
	case fac <= 1.1 && fac >= 0.8:
		// Try to free memory and check back soon
		Interval = 2 * time.Second
		log.Info("Specified memory limit reached",
			"MemoryAllocated", bytefmt.ByteSize(alloc),
			"MemoryUsed", bytefmt.ByteSize(used),
			"MemoryAvailable", bytefmt.ByteSize(avail),
			"MemoryLimit", bytefmt.ByteSize(memlimit),
			"MemoryUsagePercent", fmt.Sprintf("%.2f", fac))
		go tryFreeMemory()
	case fac < 0.8:
		// All good, let a waiting goroutine run
		Interval = DefaultInterval
		for i, goroutine := range queue {
			if goroutine.memoryRequest < avail {
				activeGoRoutines = append(activeGoRoutines, goroutine)
				queue = append(queue[:i], queue[i+1:]...)
				memtrack += goroutine.memoryRequest
				close(goroutine.waitCh)
				break
			}
		}
	default:
		panic("This should not be mathemically possible")
	}
	runmx.Unlock()
}

// tryFreeMemory calls golang runtime functions to attempt to reclaim memory
func tryFreeMemory() {
	runtime.GC()
	debug.FreeOSMemory()
}

// shedLoad kills an active go routine.
// Currently it kills the last activated one.
func shedLoad() {
	if len(activeGoRoutines) > 0 {
		killGoRoutine := activeGoRoutines[len(activeGoRoutines)-1]
		activeGoRoutines = activeGoRoutines[:len(activeGoRoutines)-1]
		killGoRoutine.cancelFnc()
		tryFreeMemory()
	}
}
