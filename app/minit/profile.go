package minit

import (
	"fmt"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"time"
)

const (
	EnvEnableProfiling = "MEFS_PROF"
	cpuProfile         = "mefs.cpuprof"
	heapProfile        = "mefs.memprof"
)

func ProfileIfEnabled() (func(), error) {
	if os.Getenv(EnvEnableProfiling) != "" {
		stopProfilingFunc, err := startProfiling() // TODO maybe change this to its own option... profiling makes it slower.
		if err != nil {
			return nil, err
		}
		return stopProfilingFunc, nil
	}
	return func() {}, nil
}

func startProfiling() (func(), error) {
	// start CPU profiling as early as possible
	ofi, err := os.Create(cpuProfile)
	if err != nil {
		return nil, err
	}
	err = pprof.StartCPUProfile(ofi)
	if err != nil {
		fmt.Println("pprof.StartCPUProfile(ofi) falied: ", err)
	}
	go func() {
		for range time.NewTicker(time.Second * 30).C {
			err := writeHeapProfileToFile()
			if err != nil {
				fmt.Println(err)
			}
		}
	}()

	stopProfiling := func() {
		pprof.StopCPUProfile()
		err = ofi.Close() // captured by the closure
		if err != nil {
			fmt.Println("ofi.Close() falied: ", err)
		}
	}
	return stopProfiling, nil
}

func writeHeapProfileToFile() error {
	mprof, err := os.Create(heapProfile)
	if err != nil {
		return err
	}
	defer mprof.Close() // _after_ writing the heap profile
	return pprof.WriteHeapProfile(mprof)
}
