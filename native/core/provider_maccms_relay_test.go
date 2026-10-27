package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 模拟公共中继代理（参照 streamhub 的 codetabs / allorigins）：
// 返回一段 MacCMS JSON，验证 fetchProviderTextViaCFRelay 能取回正文。
func TestFetchProviderTextViaCFRelay(t *testing.T) {
	const canned = `{"code":1,"list":[{"vod_id":40584,"vod_name":"仙武帝尊"}]}`
	relayHit := 0
	relay := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		relayHit++
		// 目标被 URL 编码后挂在 prefix 之后；url.QueryEscape 保留字母数字与「.」，
		// 故编码后的 query 里仍能找到 "videolist"（"ac=" 会变成 "ac%3D"）。
		if !strings.Contains(request.URL.RawQuery, "videolist") {
			t.Errorf("中继未收到编码后的目标，rawquery=%q", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(canned))
	}))
	defer relay.Close()

	// 临时把中继链指向本地测试服务器（prefix 拼 url.QueryEscape(target)），避免打真实公网。
	saved := cfRelayProxies
	cfRelayProxies = []cfRelayProxy{{prefix: relay.URL + "/relay?", label: "local-relay"}}
	defer func() { cfRelayProxies = saved }()

	d := &Downloader{client: relay.Client(), limiter: newRequestLimiter(1, 0)}
	target := "https://cfkua.wokaotianshi.eu.org/https://subocj.com/api.php/provide/vod/?ac=videolist&pg=1"
	text, err := d.fetchProviderTextViaCFRelay(context.Background(), target, "https://example/")
	if err != nil {
		t.Fatalf("中继取数失败：%v", err)
	}
	if text != canned {
		t.Fatalf("中继正文不符：got %q want %q", text, canned)
	}
	if relayHit != 1 {
		t.Fatalf("中继命中次数=%d，应为 1", relayHit)
	}
}

// allorigins/get 的 {contents:"..."} / {content:"..."} 包装应被拆开。
func TestUnwrapRelayBody(t *testing.T) {
	if got := unwrapRelayBody(`{"contents":"{\"code\":1}"}`, true); got != `{"code":1}` {
		t.Fatalf("contents 包装未拆：%q", got)
	}
	if got := unwrapRelayBody(`{"content":"hello"}`, true); got != "hello" {
		t.Fatalf("content 包装未拆：%q", got)
	}
	if got := unwrapRelayBody(`plain`, true); got != `plain` {
		t.Fatalf("非 JSON 不应改动：%q", got)
	}
	if got := unwrapRelayBody(`{"a":1}`, false); got != `{"a":1}` {
		t.Fatalf("非包装源不应改动：%q", got)
	}
}

// isCFChallengeError 只认「站点要求浏览器验证」文案（catalogResponseError 生成）。
func TestIsCFChallengeError(t *testing.T) {
	if !isCFChallengeError(errors.New("cfkua.wokaotianshi.eu.org HTTP 403：站点要求浏览器验证，当前请求无法通过")) {
		t.Fatal("CF 挑战错误未被识别")
	}
	if isCFChallengeError(errors.New("cfkua.wokaotianshi.eu.org HTTP 503")) {
		t.Fatal("非 CF 挑战误判")
	}
	if isCFChallengeError(nil) {
		t.Fatal("nil 不应为 CF 挑战")
	}
}

// 红果源不受 Cloudflare 403 影响，须被显式排除在 CF 中继之外：
// isHongguoProviderURL 只认红果的已知 host，其它源（含拼接加速源）不命中。
func TestIsHongguoProviderURL(t *testing.T) {
	for _, u := range []string{
		"https://hongguoduanju.com/detail?series_id=123",
		"https://www.hongguoduanju.com/category/drama",
		"https://djapi.999888456.xyz/api/hongguo/play?id=abc",
		"http://HONGGUODUANJU.COM/xxx", // 大小写不敏感
	} {
		if !isHongguoProviderURL(u) {
			t.Fatalf("红果 host 未被识别为红果：%s", u)
		}
	}
	for _, u := range []string{
		"https://cfkua.wokaotianshi.eu.org/https://subocj.com/api.php/provide/vod/",
		"https://example.com/detail?series_id=123",
		"https://hongguoduanju.com.evil.com/", // 仿冒 host，不应误判
		"",
	} {
		if isHongguoProviderURL(u) {
			t.Fatalf("非红果 host 被误判为红果：%s", u)
		}
	}
}

// 直连成功（非 CF 挑战、非拼接源）不应触碰中继：把中继链临时指向一个
// 「中毒」端点（一旦被调用就计数），断言直连成功后该端点 0 次命中。
func TestFetchProviderTextNoRelayOnSuccess(t *testing.T) {
	const body = `{"code":1,"list":[]}`
	// 直连端点：返回 200 JSON。
	direct := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(body))
	}))
	defer direct.Close()
	// 中继端点：若被误调用则计数（本应 0）。
	relayHit := 0
	relay := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		relayHit++
		_, _ = writer.Write([]byte("SHOULD_NOT_BE_CALLED"))
	}))
	defer relay.Close()

	saved := cfRelayProxies
	cfRelayProxies = []cfRelayProxy{{prefix: relay.URL + "/relay?", label: "poison-relay"}}
	defer func() { cfRelayProxies = saved }()

	d := &Downloader{client: direct.Client(), limiter: newRequestLimiter(1, 0)}
	// 目标非拼接源、直连 200 成功：fetchProviderText 走原逻辑，不进 CF 中继分支。
	text, err := d.fetchProviderText(context.Background(), direct.URL+"/api.php/provide/vod/?ac=videolist&pg=1", "")
	if err != nil {
		t.Fatalf("直连成功路径取数失败：%v", err)
	}
	if text != body {
		t.Fatalf("正文不符：%q", text)
	}
	if relayHit != 0 {
		t.Fatalf("直连成功却误触中继 %d 次", relayHit)
	}
}
