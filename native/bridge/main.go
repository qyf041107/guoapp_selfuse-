package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"duanjuapp/native/core"
	"os"
	"path/filepath"
	"unsafe"
)

// stderrLogFile 必须保持引用：一旦被 GC 回收，finalizer 会关掉这个文件句柄，
// 之后 fatal 就没地方写了。
var stderrLogFile *os.File

//export DuanjuInstallStderrLog
func DuanjuInstallStderrLog(logPath *C.char) int32 {
	if logPath == nil {
		return -1
	}
	target := C.GoString(logPath)
	// 路径按 UTF-8 交给 Go 处理，不会被 Windows 的 ANSI 代码页破坏
	// （数据目录常含中文用户名）。
	if dir := filepath.Dir(target); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return -1
		}
	}
	file, err := os.OpenFile(
		target,
		os.O_WRONLY|os.O_CREATE|os.O_APPEND,
		0o644,
	)
	if err != nil {
		return -1
	}
	// 把标准错误指过去。Go runtime 的 fatal error（如并发 map 读写）会把堆栈
	// 打到 stderr 然后退出，Windows 桌面端双击启动没有 console，这些输出会彻底
	// 丢失，表现为「闪退却查不到任何日志」。重定向后即便 fatal 也能留下堆栈。
	// 平台差异全部封装在 attachStderr 里（windows / 非 windows 两个文件）。
	if err := attachStderr(file); err != nil {
		_ = file.Close()
		return -1
	}
	stderrLogFile = file
	return 0
}

//export DuanjuRequest
func DuanjuRequest(input *C.char) *C.char {
	if input == nil {
		return C.CString(`{"ok":false,"error":"请求为空"}`)
	}
	return C.CString(core.NativeRequest(C.GoString(input)))
}

//export DuanjuFree
func DuanjuFree(value *C.char) { C.free(unsafe.Pointer(value)) }

func main() {}
