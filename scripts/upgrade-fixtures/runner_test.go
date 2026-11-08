package main

import (
	"strings"
	"testing"
)

func TestBuildRunnerRejectsMissingDeltaBeforeReadingSource(t *testing.T) {
	for _, tag := range []string{"v1.8.1", "v2.0.0", "../v1.8.0", ""} {
		_, err := buildRunner(t.TempDir(), t.TempDir(), tag)
		if err == nil || !strings.Contains(err.Error(), "seed delta") {
			t.Fatalf("release %q without a declared delta was accepted: %v", tag, err)
		}
	}
}
