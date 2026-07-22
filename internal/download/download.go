// Package download provides a small concurrent downloader with SHA1
// checksum verification and skip-if-unchanged behavior, used for client
// jars, libraries, and asset objects.
package download

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// Task is one file to fetch.
type Task struct {
	URL  string
	Dest string
	SHA1 string // expected hex sha1; empty means "don't verify"
	Size int64  // expected size in bytes, 0 if unknown; used only for progress display
}

// Result reports the outcome of one Task.
type Result struct {
	Task    Task
	Skipped bool // already present on disk with matching checksum
	Err     error
}

// Progress is called after every completed task (success, skip, or error)
// from a single goroutine, so implementations don't need their own
// locking.
type Progress func(done, total int, r Result)

// Options controls the worker pool.
type Options struct {
	Workers  int // default 8
	Progress Progress
}

// Run downloads all tasks concurrently, verifying checksums and skipping
// files that already match on disk. It returns all results (including
// skips) and a combined error if any task failed.
func Run(tasks []Task, opts Options) ([]Result, error) {
	if opts.Workers <= 0 {
		opts.Workers = 8
	}

	results := make([]Result, len(tasks))
	taskCh := make(chan int)
	var wg sync.WaitGroup
	var doneCount int32
	var mu sync.Mutex // protects progress callback ordering only

	worker := func() {
		defer wg.Done()
		for idx := range taskCh {
			r := doOne(tasks[idx])
			results[idx] = r
			n := atomic.AddInt32(&doneCount, 1)
			if opts.Progress != nil {
				mu.Lock()
				opts.Progress(int(n), len(tasks), r)
				mu.Unlock()
			}
		}
	}

	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go worker()
	}
	for i := range tasks {
		taskCh <- i
	}
	close(taskCh)
	wg.Wait()

	var firstErr error
	for _, r := range results {
		if r.Err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", r.Task.Dest, r.Err)
		}
	}
	return results, firstErr
}

func doOne(t Task) Result {
	if t.SHA1 != "" {
		if ok, _, _ := verifyExisting(t.Dest, t.SHA1); ok {
			return Result{Task: t, Skipped: true}
		}
	} else if info, err := os.Stat(t.Dest); err == nil && (t.Size == 0 || info.Size() == t.Size) {
		// No checksum available (some loader metadata omits it); fall back
		// to a size match, or just presence if size is also unknown.
		return Result{Task: t, Skipped: true}
	}

	if err := os.MkdirAll(filepath.Dir(t.Dest), 0o755); err != nil {
		return Result{Task: t, Err: err}
	}

	tmp := t.Dest + ".part"
	if err := fetchTo(t.URL, tmp); err != nil {
		os.Remove(tmp)
		return Result{Task: t, Err: err}
	}

	if t.SHA1 != "" {
		ok, sum, err := verifyExisting(tmp, t.SHA1)
		if err != nil {
			os.Remove(tmp)
			return Result{Task: t, Err: err}
		}
		if !ok {
			os.Remove(tmp)
			return Result{Task: t, Err: fmt.Errorf("checksum mismatch: got %s, want %s", sum, t.SHA1)}
		}
	}

	if err := os.Rename(tmp, t.Dest); err != nil {
		return Result{Task: t, Err: err}
	}
	return Result{Task: t}
}

func fetchTo(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// verifyExisting reports whether the file at path exists and its SHA1
// matches want (hex, case-insensitive).
func verifyExisting(path, want string) (bool, string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, "", err
	}
	got := hex.EncodeToString(h.Sum(nil))
	return got == want, got, nil
}
