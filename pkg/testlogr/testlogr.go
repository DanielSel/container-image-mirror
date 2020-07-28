/*
	Implementation of the logr interface from github.com/go-logr/logr
	using the golang standard library testing package.
*/
package testlogr

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
)

func NewTestLogger(t *testing.T) logr.Logger {
	return &TestLogger{
		t: t,
	}
}

// TestLogger is a logr implementation using the
// golang standard testing.Log implementation.
type TestLogger struct {
	name                  string
	loglvl, loglvlSetting uint8
	keyValues             map[string]interface{}

	t *testing.T
}

var _ logr.Logger = &TestLogger{}

func (l *TestLogger) Info(msg string, kvs ...interface{}) {
	if !l.Enabled() {
		return
	}

	logMsg := fmt.Sprintf("%s\t%s\t", l.name, msg)
	for k, v := range l.keyValues {
		logMsg += fmt.Sprintf("%s: %+v  ", k, v)
	}
	for i := 0; i < len(kvs); i += 2 {
		logMsg += fmt.Sprintf("%s: %+v  ", kvs[i], kvs[i+1])
	}
	logMsg += "\n"
	l.t.Log(logMsg)
}

func (l *TestLogger) Enabled() bool {
	return l.loglvl <= l.loglvlSetting
}

func (l *TestLogger) Error(err error, msg string, kvs ...interface{}) {
	kvs = append(kvs, "error", err)
	l.Info(msg, kvs...)
}

func (l *TestLogger) V(level int) logr.InfoLogger {
	return &TestLogger{
		name:          l.name,
		loglvl:        uint8(level),
		loglvlSetting: l.loglvlSetting,
		keyValues:     l.keyValues,
		t:             l.t,
	}
}

func (l *TestLogger) WithName(name string) logr.Logger {
	return &TestLogger{
		name:          l.name + "." + name,
		loglvl:        l.loglvl,
		loglvlSetting: l.loglvlSetting,
		keyValues:     l.keyValues,
		t:             l.t,
	}
}

func (l *TestLogger) WithValues(kvs ...interface{}) logr.Logger {
	newMap := make(map[string]interface{}, len(l.keyValues)+len(kvs)/2)
	for k, v := range l.keyValues {
		newMap[k] = v
	}
	for i := 0; i < len(kvs); i += 2 {
		newMap[kvs[i].(string)] = kvs[i+1]
	}
	return &TestLogger{
		name:          l.name,
		loglvl:        l.loglvl,
		loglvlSetting: l.loglvlSetting,
		keyValues:     newMap,
		t:             l.t,
	}
}

func (l *TestLogger) SetLogLevel(lvl uint8) {
	l.loglvlSetting = lvl
}
