package core

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/http2"
)

func hongguoALPNHost(host string) bool {
	host = strings.ToLower(host)
	if host == "djapi.999888456.xyz" {
		return true
	}
	for _, domain := range []string{"fqnovel.com", "snssdk.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
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
		defer transport.CloseIdleConnections()
		upstream := request.Clone(request.Context())
		if request.GetBody != nil {
			body, err := request.GetBody()
			if err != nil {
				return nil, err
			}
			upstream.Body = body
		}
		response, err := transport.RoundTrip(upstream)
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
