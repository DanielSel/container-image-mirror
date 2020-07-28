package cim

import (
	"log"

	"github.com/go-logr/logr"
)

// WrapStdLogger wraps a github.com/go-logr/logr into a log.Logger
// from the golang standard library
func WrapStdLogger(logger logr.InfoLogger) *log.Logger {
	return log.New(&StdLogWriter{log: logger}, "", log.LstdFlags)
}

// StdLogger represents an abstraction to wrap a logr logger
// into a io.Writer. We use it to integrate golang std log.Logger
// logs from github.com/google/go-containerregistry
type StdLogWriter struct {
	log logr.InfoLogger
}

func (s *StdLogWriter) Write(p []byte) (n int, err error) {
	s.log.Info(string(p))
	return len(p), nil
}
