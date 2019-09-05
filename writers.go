package main

import (
	"io"
	"sync"
)

// WriteCounter contains an embedded io.Writer and counts all writes.
type WriteCounter struct {
	io.Writer
	n int64
}

// NewWriteCounter creates a new *WriteCounter
func NewWriteCounter(w io.Writer) *WriteCounter {
	return &WriteCounter{w, 0}
}

// Write writes len(p) bytes from p to the underlying data stream.
// It returns the number of bytes written from p (0 <= n <= len(p))
// and any error encountered that caused the write to stop early.
// Write must return a non-nil error if it returns n < len(p).
// Write must not modify the slice data, even temporarily.
func (wc *WriteCounter) Write(p []byte) (n int, err error) {
	n, err = wc.Writer.Write(p)
	wc.n += int64(n)
	return
}

// GetCounter returns the current write counter
func (wc *WriteCounter) GetCounter() int64 {
	return wc.n
}

// LockedWriter prevents concurrent writes
type LockedWriter struct {
	io.Writer
	lock sync.Mutex
}

// NewLockedWriter creates a new *LockedWriter
func NewLockedWriter(w io.Writer) *LockedWriter {
	return &LockedWriter{Writer: w}
}

// Write writes len(p) bytes from p to the underlying data stream.
// It returns the number of bytes written from p (0 <= n <= len(p))
// and any error encountered that caused the write to stop early.
// Write must return a non-nil error if it returns n < len(p).
// Write must not modify the slice data, even temporarily.
func (lw *LockedWriter) Write(p []byte) (n int, err error) {
	lw.lock.Lock()
	defer lw.lock.Unlock()
	return lw.Writer.Write(p)
}
