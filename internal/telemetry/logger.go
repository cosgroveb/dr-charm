package telemetry

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Logger handles game session logging
type Logger struct {
	enabled    bool
	file       *os.File
	writer     *bufio.Writer
	logDir     string
	currentLog string
}

// NewLogger creates a new logger
func NewLogger(logDir string) *Logger {
	return &Logger{
		logDir:  logDir,
		enabled: false,
	}
}

// Start begins logging to a new file
func (l *Logger) Start(character string) error {
	if l.enabled {
		l.Stop()
	}

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(l.logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s_%s.log", character, timestamp)
	filepath := filepath.Join(l.logDir, filename)

	// Open file
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	l.file = file
	l.writer = bufio.NewWriter(file)
	l.currentLog = filepath
	l.enabled = true

	// Write header
	l.writeHeader(character)

	return nil
}

// Stop closes the current log file
func (l *Logger) Stop() {
	if !l.enabled || l.file == nil {
		return
	}

	l.writeFooter()
	l.writer.Flush()
	l.file.Close()

	l.file = nil
	l.writer = nil
	l.enabled = false
}

// Log writes a line to the log file
func (l *Logger) Log(line string) {
	if !l.enabled || l.writer == nil {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	l.writer.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, line))
}

// LogRaw writes raw text without timestamp
func (l *Logger) LogRaw(text string) {
	if !l.enabled || l.writer == nil {
		return
	}

	l.writer.WriteString(text)
}

// LogCommand logs a player command
func (l *Logger) LogCommand(cmd string) {
	if !l.enabled {
		return
	}

	l.Log(fmt.Sprintf("> %s", cmd))
}

// LogGameOutput logs game output
func (l *Logger) LogGameOutput(output string) {
	if !l.enabled {
		return
	}

	// Remove ANSI codes for clean logs
	clean := stripANSI(output)
	l.Log(clean)
}

// Flush ensures all buffered data is written
func (l *Logger) Flush() {
	if l.writer != nil {
		l.writer.Flush()
	}
}

// IsEnabled returns whether logging is active
func (l *Logger) IsEnabled() bool {
	return l.enabled
}

// GetCurrentLog returns the path to the current log file
func (l *Logger) GetCurrentLog() string {
	return l.currentLog
}

// writeHeader writes the log file header
func (l *Logger) writeHeader(character string) {
	header := fmt.Sprintf(`DragonRealms Session Log
========================
Character: %s
Started: %s
Client: dr-charm

`, character, time.Now().Format("2006-01-02 15:04:05"))

	l.writer.WriteString(header)
}

// writeFooter writes the log file footer
func (l *Logger) writeFooter() {
	footer := fmt.Sprintf(`
========================
Session ended: %s
`, time.Now().Format("2006-01-02 15:04:05"))

	l.writer.WriteString(footer)
}

// stripANSI removes ANSI escape codes from text
func stripANSI(text string) string {
	// Simple ANSI stripping - can be enhanced
	var result strings.Builder
	inEscape := false

	for _, ch := range text {
		if ch == '\x1b' {
			inEscape = true
		} else if inEscape {
			if ch == 'm' {
				inEscape = false
			}
		} else {
			result.WriteRune(ch)
		}
	}

	return result.String()
}

// SessionReplay handles replaying logged sessions
type SessionReplay struct {
	lines      []string
	currentIdx int
	speed      time.Duration
}

// LoadSession loads a log file for replay
func LoadSession(logPath string) (*SessionReplay, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	return &SessionReplay{
		lines:      lines,
		currentIdx: 0,
		speed:      50 * time.Millisecond, // Default replay speed
	}, nil
}

// NextLine returns the next line in the replay
func (sr *SessionReplay) NextLine() (string, bool) {
	if sr.currentIdx >= len(sr.lines) {
		return "", false
	}

	line := sr.lines[sr.currentIdx]
	sr.currentIdx++
	return line, true
}

// Reset restarts the replay from the beginning
func (sr *SessionReplay) Reset() {
	sr.currentIdx = 0
}

// SetSpeed changes the replay speed
func (sr *SessionReplay) SetSpeed(speed time.Duration) {
	sr.speed = speed
}

// GetProgress returns the current replay progress
func (sr *SessionReplay) GetProgress() (current, total int) {
	return sr.currentIdx, len(sr.lines)
}
