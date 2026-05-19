// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// MeasureCmd runs cmd to completion, returning wall-clock time and peak RSS in
// bytes. peakRSS is read from the child's ProcessState after Wait() returns;
// the units of syscall.Rusage.Maxrss are platform-dependent (Linux: KB,
// macOS: bytes) and normalized here. On platforms without POSIX rusage, peakRSS
// will be 0.
//
// The error return reflects cmd.Run()'s error; non-nil err does not invalidate
// wall or peakRSS — partial measurements remain useful for diagnostics.
func MeasureCmd(cmd *exec.Cmd) (wall time.Duration, peakRSS int64, err error) {
	start := time.Now()
	err = cmd.Run()
	wall = time.Since(start)
	if ps := cmd.ProcessState; ps != nil {
		if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
			peakRSS = normalizeMaxRSS(ru.Maxrss)
		}
	}
	return wall, peakRSS, err
}

func normalizeMaxRSS(maxrss int64) int64 {
	// Linux: kilobytes. macOS: bytes. Other Unix: typically kilobytes.
	if runtime.GOOS == "darwin" {
		return maxrss
	}
	return maxrss * 1024
}
