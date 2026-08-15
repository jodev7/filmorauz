package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeFFmpeg puts a stub `ffmpeg` script at the front of PATH for the test and
// returns a cleanup func. body is the shell body of the stub; "$@" holds the
// args downloadHLSToMP4 passed, and the output path is the final one.
func fakeFFmpeg(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" + body + "\n"
	path := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub ffmpeg: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func shortenStallTimings(t *testing.T, poll, stall, max time.Duration) {
	t.Helper()
	oldPoll, oldStall, oldMax := clipHLSStallPollInterval, clipHLSStallTimeout, clipHLSPullMaxDuration
	clipHLSStallPollInterval, clipHLSStallTimeout, clipHLSPullMaxDuration = poll, stall, max
	t.Cleanup(func() {
		clipHLSStallPollInterval, clipHLSStallTimeout, clipHLSPullMaxDuration = oldPoll, oldStall, oldMax
	})
}

// A pull whose output file never grows is the exact failure that wedged three
// processing slots for over a day: ffmpeg blocked in a network read while the
// job kept heartbeating, so no watchdog ever reclaimed it.
func TestDownloadHLSToMP4_KillsStalledPull(t *testing.T) {
	shortenStallTimings(t, 20*time.Millisecond, 200*time.Millisecond, 30*time.Second)
	// Writes nothing and hangs, mimicking a dead CDN socket.
	fakeFFmpeg(t, "sleep 30")

	p := &Pipeline{}
	out := filepath.Join(t.TempDir(), "source.mp4")

	start := time.Now()
	err := p.downloadHLSToMP4(context.Background(), "job1", "https://cdn.example/index.m3u8", out)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a stall error, got nil")
	}
	if !strings.Contains(err.Error(), "stalled") {
		t.Errorf("expected stall error, got: %v", err)
	}
	// Must be killed on the stall timeout, not left to the 30s sleep.
	if elapsed > 5*time.Second {
		t.Errorf("stalled pull took %s; watchdog did not kill it", elapsed)
	}
}

// A pull that keeps writing must be left alone even past the stall window.
func TestDownloadHLSToMP4_AllowsProgressingPull(t *testing.T) {
	shortenStallTimings(t, 20*time.Millisecond, 200*time.Millisecond, 30*time.Second)
	// Grows the output past the stall window, then exits cleanly.
	fakeFFmpeg(t, `out=$(eval echo \${$#})
i=0
while [ $i -lt 25 ]; do
  echo data >> "$out"
  sleep 0.05
  i=$((i+1))
done`)

	p := &Pipeline{}
	out := filepath.Join(t.TempDir(), "source.mp4")

	if err := p.downloadHLSToMP4(context.Background(), "job2", "https://cdn.example/index.m3u8", out); err != nil {
		t.Fatalf("progressing pull should succeed, got: %v", err)
	}
	fi, err := os.Stat(out)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("expected non-empty output, stat err=%v", err)
	}
}

// The rw_timeout flag is what bounds a single hung socket read inside ffmpeg
// itself, so it must actually reach the command line.
func TestDownloadHLSToMP4_PassesRWTimeout(t *testing.T) {
	shortenStallTimings(t, 20*time.Millisecond, 5*time.Second, 30*time.Second)
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	fakeFFmpeg(t, `out=$(eval echo \${$#})
echo "$@" > `+argsFile+`
echo data > "$out"`)

	p := &Pipeline{}
	out := filepath.Join(t.TempDir(), "source.mp4")
	if err := p.downloadHLSToMP4(context.Background(), "job3", "https://cdn.example/index.m3u8", out); err != nil {
		t.Fatalf("stub pull should succeed, got: %v", err)
	}

	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read captured args: %v", err)
	}
	// 30s expressed in microseconds, as ffmpeg expects.
	if !strings.Contains(string(got), "-rw_timeout 30000000") {
		t.Errorf("expected -rw_timeout in args, got: %s", got)
	}
}
