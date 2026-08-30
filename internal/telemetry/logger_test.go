package telemetry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStartSecuresDirectoryAndFileAndSanitizesWrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	old := umask(0o022)
	defer umask(old)

	l := NewLogger(dir)
	result, err := l.Start("A/B\\C\x1b[31m")
	if err != nil {
		t.Fatal(err)
	}
	if result.Path == "" || result.Warning != nil {
		t.Fatalf("start result=%+v", result)
	}
	if err := l.Write("hello\x1b]52;c;secret\a\r\nworld"); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(); err != nil {
		t.Fatal(err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode=%o", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	if !logName.MatchString(entries[0].Name()) {
		t.Fatalf("unsafe filename %q", entries[0].Name())
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode=%o", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.ContainsAny(text, "\x1b\x00") || !strings.Contains(text, "hello\nworld") {
		t.Fatalf("unsanitized transcript %q", text)
	}
}

func TestStartRejectsSymlinkAndNonDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLogger(link).Start("hero"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink start error=%v", err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLogger(file).Start("hero"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file start error=%v", err)
	}
}

func TestStartRepairsAndPrunesClosedMatchingLogs(t *testing.T) {
	dir := t.TempDir()
	for i := range 31 {
		name := fmt.Sprintf("dr-charm_Hero_20260830T1200%02dZ_%d.log", i%60, i)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(strings.Repeat("x", 1024)), 0o644); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(int64(i), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "unrelated.log"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := NewLogger(dir).Start("Hero")
	if err != nil {
		t.Fatal(err)
	}
	if result.Warning != nil {
		t.Fatalf("warning=%v", result.Warning)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	matching := 0
	for _, entry := range entries {
		if !logName.MatchString(entry.Name()) {
			continue
		}
		matching++
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode=%o", entry.Name(), got)
		}
	}
	if matching != 30 {
		t.Fatalf("matching logs=%d, want 30", matching)
	}
	if _, err := os.Stat(filepath.Join(dir, "unrelated.log")); err != nil {
		t.Fatalf("unrelated file removed: %v", err)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("active log removed: %v", err)
	}
}

func TestPruneKeepsOversizedActiveLogWithoutLooping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dr-charm_Hero_20260830T120000Z_1.log")
	if err := os.WriteFile(path, []byte("active"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 101*1024*1024); err != nil {
		t.Fatal(err)
	}
	l := &Logger{
		logDir:  dir,
		file:    &fakeLogFile{name: path},
		enabled: true,
		path:    path,
	}
	done := make(chan error, 1)
	go func() { done <- l.pruneLocked() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("prune did not return with only an oversized active log")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("active log removed: %v", err)
	}
}

func TestStartReportsStopFailureWhenRestartingEnabledLogger(t *testing.T) {
	dir := t.TempDir()
	oldFile := &fakeLogFile{name: filepath.Join(dir, "old.log"), syncErr: errors.New("sync failed")}
	l := &Logger{
		logDir:  dir,
		file:    oldFile,
		enabled: true,
		path:    oldFile.name,
	}
	if _, err := l.Start("Hero"); err == nil || !strings.Contains(err.Error(), "sync failed") {
		t.Fatalf("restart error = %v", err)
	}
	if l.enabled || l.file != nil || l.path != "" {
		t.Fatalf("logger state after failed restart = enabled %v file %#v path %q", l.enabled, l.file, l.path)
	}
}

func TestStartReportsCleanupFailureAfterHeaderWriteFailure(t *testing.T) {
	dir := t.TempDir()
	file := &fakeLogFile{name: filepath.Join(dir, "new.log"), writeErr: errors.New("write failed"), closeErr: errors.New("close failed")}
	l := &Logger{
		logDir: dir,
		createTemp: func(string, string) (logFile, error) {
			return file, nil
		},
	}
	if _, err := l.Start("Hero"); err == nil || !strings.Contains(err.Error(), "write failed") || !strings.Contains(err.Error(), "close failed") {
		t.Fatalf("header failure error = %v", err)
	}
	if l.enabled || l.file != nil || l.path != "" {
		t.Fatalf("logger state after header failure = enabled %v file %#v path %q", l.enabled, l.file, l.path)
	}
}

func TestSafeNameMatchesGrammar(t *testing.T) {
	for _, input := range []string{"A/B", "..", "\x00", strings.Repeat("x", 200)} {
		name := "dr-charm_" + safeName(input) + "_20260830T120000Z_1.log"
		if !regexp.MustCompile(`^dr-charm_[A-Za-z0-9._-]{1,64}_[0-9]{8}T[0-9]{6}Z_[0-9]+\.log$`).MatchString(name) {
			t.Fatalf("name %q does not match grammar", name)
		}
	}
}

type fakeLogFile struct {
	name     string
	writeErr error
	syncErr  error
	closeErr error
	chmodErr error
}

func (f *fakeLogFile) WriteString(s string) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(s), nil
}

func (f *fakeLogFile) Sync() error             { return f.syncErr }
func (f *fakeLogFile) Close() error            { return f.closeErr }
func (f *fakeLogFile) Chmod(os.FileMode) error { return f.chmodErr }
func (f *fakeLogFile) Name() string            { return f.name }

// umask has no portable getter; setting it to a known value is sufficient for
// checking that explicit file and directory modes survive a permissive umask.
func umask(mask int) int {
	return syscall.Umask(mask)
}
