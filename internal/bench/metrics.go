// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import "time"

// Run is one fixture's complete bench result. Serialized as a single JSON line.
type Run struct {
	Timestamp        time.Time   `json:"timestamp"`
	GitSHA           string      `json:"git_sha"`
	Fixture          string      `json:"fixture"`
	GenWallNS        int64       `json:"gen_wall_ns"`
	GenPeakRSSBytes  int64       `json:"gen_peak_rss_bytes"`
	BuildWallNS      int64       `json:"build_wall_ns"`
	BuildPeakRSSBytes int64      `json:"build_peak_rss_bytes"`
	TotalLOC         int         `json:"total_loc"`
	FileCount        int         `json:"file_count"`
	TopFiles         []FileStats `json:"top_files"`
}

// FileStats is the LOC count for a single generated file.
type FileStats struct {
	Path string `json:"path"`
	LOC  int    `json:"loc"`
}
