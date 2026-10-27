package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

const diagnosticLogBytes = 5 * 1024 * 1024

const diagnosticLogBackups = 2

type diagnosticEvent struct {
	HTTPStatus         int       `json:"httpStatus,omitempty"`
	Client             string    `json:"client,omitempty"`
	Protocol           string    `json:"protocol,omitempty"`
	CFRay              string    `json:"cfRay,omitempty"`
	ResponseType       string    `json:"responseType,omitempty"`
	RequestedQuality   int       `json:"requestedQuality,omitempty"`
	Time               time.Time `json:"time"`
	Level              string    `json:"level"`
	Event              string    `json:"event"`
	Source             string    `json:"source,omitempty"`
	DramaID            string    `json:"dramaId,omitempty"`
	DramaTitle         string    `json:"dramaTitle,omitempty"`
	Episode            int       `json:"episode,omitempty"`
	Run                uint64    `json:"run,omitempty"`
	StartSeconds       float64   `json:"startSeconds,omitempty"`
	Host               string    `json:"host,omitempty"`
	OffsetBytes        int64     `json:"offsetBytes,omitempty"`
	TotalBytes         int64     `json:"totalBytes,omitempty"`
	Attempt            int       `json:"attempt,omitempty"`
	Quality            int       `json:"quality,omitempty"`
	AvailableQualities []int     `json:"availableQualities,omitempty"`
	Message            string    `json:"message"`
}

type diagnosticLog struct {
	mu          sync.Mutex
	path        string
	limit       int64
	warningOnce sync.Once
}

func newDiagnosticLog(directory string) *diagnosticLog {
	return &diagnosticLog{path: filepath.Join(directory, "logs", "app.log"), limit: diagnosticLogBytes}
}

func (log *diagnosticLog) write(event diagnosticEvent) error {
	event.Time = time.Now()
	if event.Level == "" {
		event.Level = "error"
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	log.mu.Lock()
	defer log.mu.Unlock()
	if int64(len(data)) > log.limit {
		return errors.New("单条诊断日志过大")
	}
	if err := os.MkdirAll(filepath.Dir(log.path), 0700); err != nil {
		return err
	}
	info, err := os.Stat(log.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info != nil && info.Size()+int64(len(data)) > log.limit {
		if err := log.rotate(); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(log.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	return errors.Join(writeErr, file.Close())
}

func (log *diagnosticLog) rotate() error {
	oldest := log.path + "." + strconv.Itoa(diagnosticLogBackups)
	if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for index := diagnosticLogBackups; index > 0; index-- {
		previous := log.path
		if index > 1 {
			previous += "." + strconv.Itoa(index-1)
		}
		if err := os.Rename(previous, log.path+"."+strconv.Itoa(index)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func redactConfiguredString(cfg *Config, value string) string {
	for _, secret := range []string{cfg.Token, cfg.AESKeyHex, cfg.InterfaceKey, cfg.ParamKey, cfg.ParamIV} {
		if secret = strings.TrimSpace(secret); secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	return redactErrorString(value)
}

func (downloader *Downloader) recordDiagnostic(event diagnosticEvent) {
	if downloader == nil || downloader.diagnostics == nil {
		return
	}
	event.Message = truncate(redactConfiguredString(&downloader.cfg, event.Message), 4096)
	event.DramaTitle = truncate(redactConfiguredString(&downloader.cfg, event.DramaTitle), 256)
	if err := downloader.diagnostics.write(event); err != nil {
		downloader.diagnostics.warningOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "诊断日志保存失败：%v\n", publicError(err))
		})
	}
}

// recordGoroutinePanic 把一次 goroutine 里的 panic 堆栈落盘到 app.log（事件
// goroutine_panic，level=error）。调用方应在 defer recover 后调用；recover
// 到值后进程不再 fatal，仅该 goroutine 静默退出。
//
// 该 helper 本身对写盘失败做 recover，避免在 recover 路径上二次 panic 触发
// fatal（会抵消 recover 的保护效果）。
func (downloader *Downloader) recordGoroutinePanic(label string, r any) {
	defer func() { recover() }()
	if downloader == nil {
		return
	}
	stack := debug.Stack()
	truncated := string(stack)
	if len(truncated) > 4096 {
		truncated = truncated[:4096]
	}
	event := diagnosticEvent{
		Level:   "error",
		Event:   "goroutine_panic",
		Message: fmt.Sprintf("site=%s value=%v\n%s", label, fmt.Sprintf("%v", r), truncated),
	}
	downloader.recordDiagnostic(event)
}

// diagnosticLog 的 recover 版本：对没有 Downloader 上下文（如 LAN server
// 构造期）调用方使用，把 stack 写到 <dir>/logs/app.log。
func (log *diagnosticLog) recordPanic(label string, r any) {
	if log == nil {
		return
	}
	stack := string(debug.Stack())
	if len(stack) > 4096 {
		stack = stack[:4096]
	}
	_ = log.write(diagnosticEvent{
		Level:   "error",
		Event:   "goroutine_panic",
		Message: fmt.Sprintf("site=%s value=%v\n%s", label, fmt.Sprintf("%v", r), stack),
	})
}
