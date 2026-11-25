package daemon

import "testing"

func TestResourceLimits(t *testing.T) {
	const mib = int64(1024 * 1024)
	cases := []struct {
		name      string
		maxProcs  int
		limitMib  int
		wantProcs int
		wantBytes int64
	}{
		{"high parallelism is capped", 16, 512, 4, 512 * mib},
		{"exact cap is unchanged", 4, 128, 4, 128 * mib},
		{"low parallelism is never raised", 1, 64, 1, 64 * mib},
		{"zero parallelism is left alone", 0, 32, 0, 32 * mib},
		{"unset limit uses default", 8, 0, 4, 256 * mib},
		{"negative limit uses default", 2, -1, 2, 256 * mib},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			procs, bytes := resourceLimits(tc.maxProcs, tc.limitMib)
			if procs != tc.wantProcs || bytes != tc.wantBytes {
				t.Fatalf("resourceLimits(%d, %d) = (%d, %d), want (%d, %d)",
					tc.maxProcs, tc.limitMib, procs, bytes, tc.wantProcs, tc.wantBytes)
			}
		})
	}
}
