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
		stopProfilingFunc, err := startProfiling()
		if err != nil {
			return nil, err
		}
		return stopProfilingFunc, nil
	}
	return func() {}, nil
}

func startProfiling() (func(), error) {
	ofi, err := os.Create(cpuProfile)
	if err != nil {
		return nil, err
	}

	fmt.Println("start cpu profile")
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
		err = ofi.Close()
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
	defer mprof.Close()
	return pprof.WriteHeapProfile(mprof)
}
