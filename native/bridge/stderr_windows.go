//go:build windows

package main

import (
	"os"
	"syscall"
)

// STD_ERROR_HANDLE 的数值是 (DWORD)-12。这里写成常量而不是直接转型，
// 因为 -12 这个负常量无法直接转换成 uintptr / uint32（编译期常量溢出）。
const stdErrorHandleValue = 0xFFFFFFF4

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procSetStdHandle = modkernel32.NewProc("SetStdHandle")
)

// attachStderr 把进程的标准错误指到 f 对应的文件。
//
// 为什么用 SetStdHandle 而不是 CRT 的 _dup2：
//
//	os.File.Fd() 在 Windows 返回的是**操作系统句柄 HANDLE**，不是 CRT 文件描述符。
//	把 HANDLE（如 0x1a4）当 fd 传给 _dup2/_get_osfhandle，UCRT 的
//	invalid-parameter 检查会 invoke_watson → __fastfail，进程以 0xC0000409
//	当场退出，连启动界面都出不来，且不留任何日志（这个坑真的踩过）。
//
// 而 Go runtime 输出 fatal error（并发 map 读写等）时，对 fd 1/2 是特判成
// GetStdHandle(STD_ERROR_HANDLE) 来写的，所以只改这个值就能让 fatal 堆栈落盘，
// 完全不需要碰 CRT。
func attachStderr(f *os.File) error {
	handle := syscall.Handle(f.Fd())
	if handle == 0 || handle == syscall.InvalidHandle {
		return syscall.EINVAL
	}
	r, _, err := procSetStdHandle.Call(uintptr(stdErrorHandleValue), uintptr(handle))
	if r == 0 {
		if err == nil {
			err = syscall.EINVAL
		}
		return err
	}
	return nil
}
