package core

import (
	"crypto/x509"
	_ "embed"
)

// cacert.pem 是 Mozilla 维护的 CA 全集（随 guoapp3 一起发布，含 Certum、Let's Encrypt、
// DigiCert、GlobalSign 等所有公共根）。通过 go:embed 在编译期打进二进制，无需运行时文件系统
// 路径，因此 Android 的 .so 也能直接使用。
//
//go:embed cacert.pem
var embeddedCACerts []byte

// systemCertPoolWithBundle 返回「系统信任库 + 打包 CA 全集」合并后的证书池。
//
// 背景：本项目的 Go 引擎通过 dart:ffi 嵌入，并未引入 golang.org/x/mobile，因此 Go 的
// crypto/tls 在 Android 上不会自动加载系统信任库，x509.SystemCertPool() 在 Android 端返回
// 的是空池。此时真正为 HTTPS 提供信任锚的就是这里的 embeddedCACerts。把打包 CA 追加到系统池
// 之上，既能让 Android 信任 Certum 这类小众公共 CA（修复「certificate signed by unknown
// authority」），又不会改变 Windows / macOS / Linux / Web 上原本就依赖系统库的行为。
func systemCertPoolWithBundle() *x509.CertPool {
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	if len(embeddedCACerts) > 0 {
		pool.AppendCertsFromPEM(embeddedCACerts)
	}
	return pool
}
