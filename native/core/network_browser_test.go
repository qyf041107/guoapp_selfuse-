package core

import (
	"io"
	"net/http"
	"strings"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

// 自定义源（运行期 registerHost 的 host）与内置 Browser 源的传输选择不同：
// 内置源直接用浏览器指纹；自定义源先走标准传输，只有被 Cloudflare 验证页
// 拦住时才升级到指纹传输。下面的用例钉住这三条行为。

type browserTransportFunc func(*http.Request) (*http.Response, error)

func (f browserTransportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type stubBrowserClient struct {
	status int
	calls  int
}

func (client *stubBrowserClient) Do(*fhttp.Request) (*fhttp.Response, error) {
	client.calls++
	return &fhttp.Response{
		Status: "200 OK", StatusCode: client.status, Proto: "HTTP/2.0", ProtoMajor: 2,
		Header: fhttp.Header{}, Body: io.NopCloser(strings.NewReader("<html>ok</html>")),
	}, nil
}

func (client *stubBrowserClient) CloseIdleConnections() {}

func newTestBrowserTransport(base http.RoundTripper, client *stubBrowserClient) *huangguoBrowserTransport {
	transport := &huangguoBrowserTransport{
		base:     base,
		router:   &proxyRouter{mode: "direct"},
		hosts:    map[string]bool{"static.example.com": true},
		custom:   map[string]bool{},
		upgraded: map[string]bool{},
		clients:  map[string]browserHTTPClient{},
	}
	transport.newClient = func(string) (browserHTTPClient, error) { return client, nil }
	return transport
}

func testRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestBrowserTransportCustomHostPrefersStandardClient(t *testing.T) {
	standard := 0
	client := &stubBrowserClient{status: 200}
	transport := newTestBrowserTransport(browserTransportFunc(func(*http.Request) (*http.Response, error) {
		standard++
		return &http.Response{
			StatusCode: 200, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Header: http.Header{}, Body: io.NopCloser(strings.NewReader("<html>ok</html>")),
		}, nil
	}), client)
	transport.registerHost("www.hanjuds.com")

	response, err := transport.RoundTrip(testRequest(t, "https://www.hanjuds.com/80s/index-1-1.html"))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	response.Body.Close()
	if standard != 1 || client.calls != 0 {
		t.Fatalf("自定义源应先用标准传输：standard=%d browser=%d", standard, client.calls)
	}
	if transport.promoted(testRequest(t, "https://www.hanjuds.com/")) {
		t.Fatal("200 响应不应把 host 升级为指纹传输")
	}
}

func TestBrowserTransportCustomHostUpgradesOnChallenge(t *testing.T) {
	standard := 0
	client := &stubBrowserClient{status: 200}
	transport := newTestBrowserTransport(browserTransportFunc(func(*http.Request) (*http.Response, error) {
		standard++
		if standard == 1 {
			return &http.Response{
				StatusCode: 403, Status: "403 Forbidden", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
				Header: http.Header{"Cf-Mitigated": []string{"challenge"}},
				Body:   io.NopCloser(strings.NewReader("challenge")),
			}, nil
		}
		return &http.Response{
			StatusCode: 403, Status: "403 Forbidden", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Header: http.Header{}, Body: io.NopCloser(strings.NewReader("challenge")),
		}, nil
	}), client)
	transport.registerHost("www.hanjuds.com")

	// 第一次：标准传输被 Cloudflare 验证页拦住 → 升级并改用指纹传输
	response, err := transport.RoundTrip(testRequest(t, "https://www.hanjuds.com/80s/index-1-1.html"))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	response.Body.Close()
	if client.calls != 1 {
		t.Fatalf("撞上验证页后应改用指纹传输一次，实际 browser=%d", client.calls)
	}

	// 第二次：host 已被记住，不再浪费一次标准请求
	standardBefore := standard
	response, err = transport.RoundTrip(testRequest(t, "https://www.hanjuds.com/80s/index-2-1.html"))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	response.Body.Close()
	if standard != standardBefore || client.calls != 2 {
		t.Fatalf("已升级的 host 不该再走标准传输：standard=%d browser=%d", standard, client.calls)
	}
}

func TestBrowserTransportStaticHostUsesBrowserClient(t *testing.T) {
	standard := 0
	client := &stubBrowserClient{status: 200}
	transport := newTestBrowserTransport(browserTransportFunc(func(*http.Request) (*http.Response, error) {
		standard++
		return &http.Response{
			StatusCode: 200, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok")),
		}, nil
	}), client)

	response, err := transport.RoundTrip(testRequest(t, "https://static.example.com/api/list"))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	response.Body.Close()
	if standard != 0 || client.calls != 1 {
		t.Fatalf("内置 Browser 源应直接用指纹传输：standard=%d browser=%d", standard, client.calls)
	}
}

func TestBrowserTransportLeavesUntrackedHostAlone(t *testing.T) {
	standard := 0
	client := &stubBrowserClient{status: 200}
	transport := newTestBrowserTransport(browserTransportFunc(func(*http.Request) (*http.Response, error) {
		standard++
		return &http.Response{
			StatusCode: 200, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok")),
		}, nil
	}), client)

	response, err := transport.RoundTrip(testRequest(t, "https://other.example.com/page"))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	response.Body.Close()
	if standard != 1 || client.calls != 0 {
		t.Fatalf("未登记的 host 应走标准传输：standard=%d browser=%d", standard, client.calls)
	}
}
