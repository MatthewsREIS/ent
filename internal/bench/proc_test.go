// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMeasureCmd_SuccessfulProcess(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("MeasureCmd uses POSIX rusage; skipping on %s", runtime.GOOS)
	}
	cmd := exec.Command("sh", "-c", "sleep 0.05")
	wall, peakRSS, err := MeasureCmd(cmd)
	require.NoError(t, err)
	require.GreaterOrEqual(t, wall, 50*time.Millisecond, "wall should include the 50ms sleep")
	require.Greater(t, peakRSS, int64(0), "peak RSS must be measurable on Linux/macOS")
}

func TestMeasureCmd_FailedProcess(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("MeasureCmd uses POSIX rusage; skipping on %s", runtime.GOOS)
	}
	cmd := exec.Command("sh", "-c", "exit 7")
	wall, peakRSS, err := MeasureCmd(cmd)
	require.Error(t, err, "non-zero exit must surface as error")
	require.GreaterOrEqual(t, wall, time.Duration(0), "wall should still be reported")
	require.GreaterOrEqual(t, peakRSS, int64(0), "peakRSS may be zero on early failure but never negative")
}
