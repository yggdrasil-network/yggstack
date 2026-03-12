package mobile

import (
	"fmt"
	"sync"
)

// // // // // // // // // //

// LogCallback receives formatted log lines from the node.
// Implementations must not block.
type LogCallback interface {
	Log(message string)
}

// PeerChangeCallback is notified when the count of connected peers changes.
// Implementations must not block.
type PeerChangeCallback interface {
	OnPeerCountChanged(connected, total int64)
}

// // // // // // // // // //

const (
	logLevelTrace = 0
	logLevelDebug = 1
	logLevelInfo  = 2
	logLevelWarn  = 3
	logLevelError = 4
)

func parseLogLevel(level string) int {
	switch level {
	case "trace":
		return logLevelTrace
	case "debug":
		return logLevelDebug
	case "warn", "warning":
		return logLevelWarn
	case "error":
		return logLevelError
	default:
		return logLevelInfo
	}
}

// //

// logBridgeObj implements core.Logger and forwards to LogCallback.
type logBridgeObj struct {
	mu    sync.RWMutex
	cb    LogCallback
	level int
}

func newLogBridge() *logBridgeObj {
	return &logBridgeObj{level: logLevelInfo}
}

func (b *logBridgeObj) setCallback(cb LogCallback) {
	b.mu.Lock()
	b.cb = cb
	b.mu.Unlock()
}

func (b *logBridgeObj) setLevel(level string) {
	b.mu.Lock()
	b.level = parseLogLevel(level)
	b.mu.Unlock()
}

func (b *logBridgeObj) emit(levelVal int, prefix, msg string) {
	b.mu.RLock()
	cb := b.cb
	min := b.level
	b.mu.RUnlock()
	if cb == nil || levelVal < min {
		return
	}
	// Deliver on a separate goroutine — callback must not block the node.
	go cb.Log(prefix + msg)
}

func (b *logBridgeObj) Printf(f string, args ...interface{}) {
	b.emit(logLevelInfo, "", fmt.Sprintf(f, args...))
}
func (b *logBridgeObj) Println(args ...interface{}) { b.emit(logLevelInfo, "", fmt.Sprintln(args...)) }
func (b *logBridgeObj) Infof(f string, args ...interface{}) {
	b.emit(logLevelInfo, "[INFO]  ", fmt.Sprintf(f, args...))
}
func (b *logBridgeObj) Infoln(args ...interface{}) {
	b.emit(logLevelInfo, "[INFO]  ", fmt.Sprintln(args...))
}
func (b *logBridgeObj) Warnf(f string, args ...interface{}) {
	b.emit(logLevelWarn, "[WARN]  ", fmt.Sprintf(f, args...))
}
func (b *logBridgeObj) Warnln(args ...interface{}) {
	b.emit(logLevelWarn, "[WARN]  ", fmt.Sprintln(args...))
}
func (b *logBridgeObj) Errorf(f string, args ...interface{}) {
	b.emit(logLevelError, "[ERROR] ", fmt.Sprintf(f, args...))
}
func (b *logBridgeObj) Errorln(args ...interface{}) {
	b.emit(logLevelError, "[ERROR] ", fmt.Sprintln(args...))
}
func (b *logBridgeObj) Debugf(f string, args ...interface{}) {
	b.emit(logLevelDebug, "[DEBUG] ", fmt.Sprintf(f, args...))
}
func (b *logBridgeObj) Debugln(args ...interface{}) {
	b.emit(logLevelDebug, "[DEBUG] ", fmt.Sprintln(args...))
}
func (b *logBridgeObj) Traceln(args ...interface{}) {
	b.emit(logLevelTrace, "[TRACE] ", fmt.Sprintln(args...))
}

// //

// peerBridgeObj implements peers.ChangeCallbackInterface and forwards to PeerChangeCallback.
type peerBridgeObj struct {
	mu sync.RWMutex
	cb PeerChangeCallback
}

func newPeerBridge() *peerBridgeObj {
	return &peerBridgeObj{}
}

func (b *peerBridgeObj) setCallback(cb PeerChangeCallback) {
	b.mu.Lock()
	b.cb = cb
	b.mu.Unlock()
}

// OnPeerCountChanged implements peers.ChangeCallbackInterface.
// Delivers the event on a goroutine to avoid blocking the peer monitor.
func (b *peerBridgeObj) OnPeerCountChanged(connected, total int64) {
	b.mu.RLock()
	cb := b.cb
	b.mu.RUnlock()
	if cb == nil {
		return
	}
	go cb.OnPeerCountChanged(connected, total)
}
