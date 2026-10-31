package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"golang.org/x/net/http2"
)

func hongguoALPNHost(host string) bool {
	host = strings.ToLower(host)
	for _, domain := range []string{"fqnovel.com", "snssdk.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// hongguoALPNRetryable 只接受 TLS 协议协商/握手被重置一类的失败。DNS、
// 证书、调用方取消和普通超时都不属于 ALPN 问题，换协议没有意义。
func hongguoALPNRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return false
	}
	var certificateErr *tls.CertificateVerificationError
	if errors.As(err, &certificateErr) {
		return false
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "certificate") || strings.Contains(text, "x509") {
		return false
	}
	for _, signal := range []string{
		"no application protocol",
		"tls:",
		"handshake",
		"connection reset by peer",
		"unexpected eof",
	} {
		if strings.Contains(text, signal) {
			return true
		}
	}
	return false
}

func hongguoALPNErrorLabel(err error) string {
	if err == nil {
		return "none"
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "no application protocol"):
		return "no_application_protocol"
	case strings.Contains(text, "connection reset"):
		return "connection_reset"
	case strings.Contains(text, "unexpected eof"):
		return "unexpected_eof"
	case strings.Contains(text, "handshake"):
		return "handshake"
	default:
		return "transport"
	}
}

func retryHongguoALPN(base *http.Transport, request *http.Request, original error) (*http.Response, error) {
	if request.Context().Err() != nil {
		return nil, request.Context().Err()
	}
	if request.Body != nil && request.GetBody == nil {
		return nil, original
	}
	lastErr := original
	for _, protocol := range []string{"http/1.1", "h2"} {
		transport := base.Clone()
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		if protocol == "h2" {
			transport.ForceAttemptHTTP2 = true
			transport.TLSNextProto = nil
			if err := http2.ConfigureTransport(transport); err != nil {
				lastErr = err
				continue
			}
		} else {
			transport.ForceAttemptHTTP2 = false
			transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		}
		transport.TLSClientConfig.NextProtos = []string{protocol}
		upstream := request.Clone(request.Context())
		if request.GetBody != nil {
			body, err := request.GetBody()
			if err != nil {
				transport.CloseIdleConnections()
				return nil, err
			}
			upstream.Body = body
		}
		response, err := transport.RoundTrip(upstream)
		transport.CloseIdleConnections()
		if err == nil {
			return response, nil
		}
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		lastErr = err
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
	}
	return nil, fmt.Errorf("TLS 协议协商失败（已尝试 HTTP/1.1 与 HTTP/2）: %w", lastErr)
}
