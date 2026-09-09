package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// RotatingFileWriter implements io.WriteCloser with automatic size-based log rotation.
type RotatingFileWriter struct {
	mu         sync.Mutex
	filePath   string
	maxBytes   int64
	maxBackups int
	file       *os.File
	size       int64
}

// NewRotatingFileWriter creates a new RotatingFileWriter.
// If maxBytes <= 0, defaults to 10MB (10 * 1024 * 1024).
// If maxBackups <= 0, defaults to 5 backups.
func NewRotatingFileWriter(filePath string, maxBytes int64, maxBackups int) (*RotatingFileWriter, error) {
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024 // 10 MB
	}
	if maxBackups <= 0 {
		maxBackups = 5
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %q: %w", dir, err)
	}

	w := &RotatingFileWriter{
		filePath:   filePath,
		maxBytes:   maxBytes,
		maxBackups: maxBackups,
	}

	if err := w.openFile(); err != nil {
		return nil, err
	}

	return w, nil
}

func (w *RotatingFileWriter) openFile() error {
	info, err := os.Stat(w.filePath)
	if err == nil {
		w.size = info.Size()
	} else if os.IsNotExist(err) {
		w.size = 0
	} else {
		return err
	}

	file, err := os.OpenFile(w.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	w.file = file
	return nil
}

// Write writes bytes to the file, rotating when size limit is reached.
func (w *RotatingFileWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	writeLen := int64(len(p))
	if writeLen > w.maxBytes {
		return 0, fmt.Errorf("write length %d exceeds max file size %d", writeLen, w.maxBytes)
	}

	if w.file == nil {
		if err := w.openFile(); err != nil {
			return 0, err
		}
	}

	if w.size+writeLen > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate closes the current file, shifts backup files, and opens a fresh one.
func (w *RotatingFileWriter) rotate() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}

	// Shift existing backups: file.4 -> file.5, file.3 -> file.4, ...
	for i := w.maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", w.filePath, i)
		dst := fmt.Sprintf("%s.%d", w.filePath, i+1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}

	// Move active file to file.1
	firstBackup := fmt.Sprintf("%s.1", w.filePath)
	_ = os.Rename(w.filePath, firstBackup)

	w.size = 0
	return w.openFile()
}

// Close closes the underlying open file descriptor.
func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// Ensure RotatingFileWriter implements io.WriteCloser
var _ io.WriteCloser = (*RotatingFileWriter)(nil)
