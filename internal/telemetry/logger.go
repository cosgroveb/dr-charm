package telemetry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"dr-charm/internal/terminaltext"
)

var logName = regexp.MustCompile(`^dr-charm_[A-Za-z0-9._-]{1,64}_[0-9]{8}T[0-9]{6}Z_[0-9]+\.log$`)

type Logger struct {
	mu         sync.Mutex
	enabled    bool
	file       logFile
	logDir     string
	path       string
	createTemp func(string, string) (logFile, error)
}

type StartResult struct {
	Path    string
	Warning error
}

type logFile interface {
	WriteString(string) (int, error)
	Sync() error
	Close() error
	Chmod(os.FileMode) error
	Name() string
}

func NewLogger(logDir string) *Logger { return &Logger{logDir: logDir} }
func (l *Logger) Start(character string) (StartResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.enabled {
		if err := l.stopLocked(); err != nil {
			return StartResult{}, err
		}
	}
	if info, err := os.Lstat(l.logDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return StartResult{}, fmt.Errorf("log directory is a symlink")
		}
		if !info.IsDir() {
			return StartResult{}, fmt.Errorf("log path is not a directory")
		}
	} else if !os.IsNotExist(err) {
		return StartResult{}, fmt.Errorf("inspect log directory: %w", err)
	}
	if err := os.MkdirAll(l.logDir, 0o700); err != nil {
		return StartResult{}, fmt.Errorf("create log directory: %w", err)
	}
	if err := os.Chmod(l.logDir, 0o700); err != nil {
		return StartResult{}, fmt.Errorf("secure log directory: %w", err)
	}
	safe := safeName(character)
	stamp := time.Now().UTC().Format("20060102T150405Z")
	f, err := l.openTemp("dr-charm_" + safe + "_" + stamp + "_*.log")
	if err != nil {
		return StartResult{}, fmt.Errorf("create log file: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		return StartResult{}, errors.Join(fmt.Errorf("secure log file: %w", err), f.Close())
	}
	l.file = f
	l.enabled = true
	l.path = f.Name()
	if err := l.writeLocked(fmt.Sprintf("DragonRealms Session Log\nCharacter: %s\nStarted: %s\n\n", terminaltext.Sanitize(character), time.Now().UTC().Format(time.RFC3339))); err != nil {
		closeErr := l.stopLocked()
		return StartResult{}, errors.Join(err, closeErr)
	}
	warning := l.pruneLocked()
	return StartResult{Path: l.path, Warning: warning}, nil
}

func (l *Logger) openTemp(pattern string) (logFile, error) {
	if l.createTemp != nil {
		return l.createTemp(l.logDir, pattern)
	}
	return os.CreateTemp(l.logDir, pattern)
}
func (l *Logger) Stop() error { l.mu.Lock(); defer l.mu.Unlock(); return l.stopLocked() }
func (l *Logger) stopLocked() error {
	if !l.enabled || l.file == nil {
		return nil
	}
	err := errors.Join(
		l.writeLocked(fmt.Sprintf("\nSession ended: %s\n", time.Now().UTC().Format(time.RFC3339))),
		l.file.Sync(),
		l.file.Close(),
	)
	l.file = nil
	l.path = ""
	l.enabled = false
	return err
}
func (l *Logger) Write(line string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.enabled {
		return nil
	}
	return l.writeLocked(fmt.Sprintf("[%s] %s\n", time.Now().UTC().Format("15:04:05"), terminaltext.Sanitize(line)))
}
func (l *Logger) writeLocked(s string) error {
	if l.file == nil {
		return nil
	}
	_, err := l.file.WriteString(terminaltext.Sanitize(s))
	return err
}
func (l *Logger) IsEnabled() bool { l.mu.Lock(); defer l.mu.Unlock(); return l.enabled }
func (l *Logger) Path() string    { l.mu.Lock(); defer l.mu.Unlock(); return l.path }
func safeName(s string) string {
	var b strings.Builder
	for _, r := range terminaltext.Sanitize(s) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._-", r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	v := b.String()
	if v == "" {
		v = "character"
	}
	if len(v) > 64 {
		v = v[:64]
	}
	return v
}
func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}
func (l *Logger) pruneLocked() error {
	entries, err := os.ReadDir(l.logDir)
	var warning error
	warning = firstErr(warning, err)
	type item struct {
		path string
		info os.FileInfo
	}
	var files []item
	for _, e := range entries {
		if !logName.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			warning = firstErr(warning, err)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(l.logDir, e.Name())
		warning = firstErr(warning, os.Chmod(path, 0o600))
		files = append(files, item{path: path, info: info})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].info.ModTime().Before(files[j].info.ModTime()) })
	total := int64(0)
	for _, f := range files {
		total += f.info.Size()
	}
	for len(files) > 30 || total > 100*1024*1024 {
		if len(files) == 0 {
			break
		}
		if l.file != nil && files[0].path == l.file.Name() {
			if len(files) == 1 {
				break
			}
			files = append(files[1:], files[0])
			continue
		}
		warning = firstErr(warning, os.Remove(files[0].path))
		total -= files[0].info.Size()
		files = files[1:]
	}
	return warning
}
