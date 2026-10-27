package core

// 本文件回归 wrxbpq 移植的 poster-items 模板家族能力（555yx.vip / qqys01.com 实测形态）：
// 粘连详情 ID、player_data 双层编码解密、module-play-list-content 分集折叠。
// 全部离线：喂磁盘样本字符串，不联网。

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestMaccmsGluedDetailSourceID(t *testing.T) {
	cases := map[string]string{
		"https://www.qqys01.com/voddetail12543.html": "12543",
		"https://www.qqys01.com/detail32.html":       "32",
		// 斜杠形态（既有能力）不得回归：
		"https://www.example.cn/voddetail/452795.html": "452795",
		// 播放页 {id}-{sid}-{nid} 不得被粘连规则误提取：
		"https://www.qqys01.com/vodplay/12543-1-1.html": "",
	}
	for link, want := range cases {
		if got := maccmsSourceIDFromURL(link); got != want {
			t.Fatalf("maccmsSourceIDFromURL(%q) = %q, want %q", link, got, want)
		}
	}
}

func TestMaccmsPosterItemsGluedCatalog(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`<html><body>
	<div class="module-main"><div class="module-page">
	<a href="/voddetail12543.html" title="超级少女" class="module-poster-item module-item">
	  <div class="module-item-pic"><img data-original="/upload/p1.jpg" class="lazy"></div>
	  <div class="module-item-title">超级少女</div>
	  <div class="module-item-note">HD</div></a>
	<a href="/voddetail31722.html" title="第三者" class="module-poster-item module-item">
	  <div class="module-item-pic"><img data-original="/upload/p2.jpg" class="lazy"></div>
	  <div class="module-item-title">第三者</div>
	  <div class="module-item-note">更新至8集</div></a>
	<a href="/voddetail43045.html" title="再战明天" class="module-poster-item module-item">
	  <div class="module-item-pic"><img data-original="/upload/p3.jpg" class="lazy"></div>
	  <div class="module-item-title">再战明天</div>
	  <div class="module-item-note">完结</div></a>
	</div></div></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	const base = "https://www.qqys01.com"
	items := maccmsCards(document, "custom:abcd1234", base)
	if len(items) != 3 {
		t.Fatalf("glued poster cards = %d, want 3: %+v", len(items), items)
	}
	if items[0].SourceID != "12543" || items[1].SourceID != "31722" || items[2].SourceID != "43045" {
		t.Fatalf("glued source ids wrong: %+v", items)
	}
	if items[0].Cover != base+"/upload/p1.jpg" {
		t.Fatalf("poster cover wrong: %q", items[0].Cover)
	}
	if items[0].DisplayTitle() != "超级少女" {
		t.Fatalf("poster title wrong: %q", items[0].DisplayTitle())
	}
	if items[1].Remark != "更新至8集" {
		t.Fatalf("poster remark wrong: %q", items[1].Remark)
	}
	// 粘连链接也必须被详情锚点识别（maccmsDetailLink 兜底通用抽取依赖它）。
	if !maccmsDetailLink.MatchString("/voddetail12543.html") {
		t.Fatal("maccmsDetailLink must accept glued /voddetail12543.html")
	}
}

func TestMaccmsPlayerDataEncrypt2DoubleEncoded(t *testing.T) {
	m3u8 := "https://cdn.example.cn/20260721/8794_fe49fbfd/index.m3u8"
	b64 := base64.StdEncoding.EncodeToString([]byte(m3u8))
	// qqys01 式：base64 外套 urlencode（% → %25 之类都会出现）。
	double := url.QueryEscape(b64)
	body := fmt.Sprintf(`<html><script>var player_data={"flag":"1","from":"lzm3u8","url":"%s","encrypt":"2"};</script></html>`, double)
	if got := maccmsPlayerURL(body); got != m3u8 {
		t.Fatalf("double-encoded player_data url = %q, want %q", got, m3u8)
	}
	// 裸数字 encrypt（不带引号）也要解析到。
	body2 := fmt.Sprintf(`<html><script>var player_data={"from":"lzm3u8","url":"%s","encrypt":2}</script>`, double)
	fields := maccmsPlayerFields(body2)
	if fields["encrypt"] != "2" {
		t.Fatalf("bare-number encrypt = %q, want 2 (fields %+v)", fields["encrypt"], fields)
	}
	if got := maccmsPlayerURL(body2); got != m3u8 {
		t.Fatalf("bare-number player_data url = %q", got)
	}
	// 纯 base64（标准 MacCMS encrypt=2）不得回归。
	bodyPlain := fmt.Sprintf(`<html><script>var player_data={"url":"%s","encrypt":"2"};</script>`, b64)
	if got := maccmsPlayerURL(bodyPlain); got != m3u8 {
		t.Fatalf("plain base64 player_data url = %q", got)
	}
	// encrypt=1 escape 形态不得回归。
	bodyEsc := `<html><script>var player_data={"url":"https%3A//cdn.example.cn/a/index.m3u8","encrypt":"1"};</script>`
	if got := maccmsPlayerURL(bodyEsc); got != "https://cdn.example.cn/a/index.m3u8" {
		t.Fatalf("encrypt=1 player_data url = %q", got)
	}
}

func TestMaccmsSearchPageRecognized(t *testing.T) {
	q := "蜘蛛"
	// 真结果页（内嵌关键词形态）：路径含 search 且 URL 带关键词 percent 形态。
	embedded := "https://www.qqys01.com/vodsearch/" + url.PathEscape(q) + "-------------.html"
	if !maccmsSearchPageRecognized(embedded, embedded, q) {
		t.Fatalf("embedded-keyword search page must be recognized: %q", embedded)
	}
	// GET wd= 形态结果页。
	getForm := "https://www.qqys01.com/vodsearch/-------------.html?wd=" + url.QueryEscape(q)
	if !maccmsSearchPageRecognized(getForm, getForm, q) {
		t.Fatal("GET wd= search page must be recognized")
	}
	// 被重定向回首页：最终 URL 无搜索段 → 未命中（即便请求 URL 带关键词）。
	if maccmsSearchPageRecognized("https://www.qqys01.com/", embedded, q) {
		t.Fatal("redirect-back-to-home page must NOT be recognized")
	}
	// 空关键词一律放行。
	if !maccmsSearchPageRecognized("https://www.qqys01.com/", "", "") {
		t.Fatal("empty query must pass")
	}
}

func TestMaccmsModulePlayListEpisodesCollapse(t *testing.T) {
	// mxone poster-items 详情页：每条线路一个 module-play-list-content 容器，
	// 链接形态 /vodplay/{id}-{sid}-{nid}.html。
	document, err := html.Parse(strings.NewReader(`<html><body>
	<div class="module-player-list"><div class="module-row-one">
	<div class="module-play-list-content  module-play-list-base " data-dropdown-value="资源1">
	  <a class="module-play-list-link" href="/vodplay/31722-1-1.html"><span>第1集</span></a>
	  <a class="module-play-list-link" href="/vodplay/31722-1-2.html"><span>第2集</span></a>
	</div>
	<div class="module-play-list-content  module-play-list-base " data-dropdown-value="资源2">
	  <a class="module-play-list-link" href="/vodplay/31722-2-1.html"><span>第1集</span></a>
	  <a class="module-play-list-link" href="/vodplay/31722-2-2.html"><span>第2集</span></a>
	  <a class="module-play-list-link" href="/vodplay/31722-2-3.html"><span>第3集</span></a>
	</div>
	</div></div></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	episodes := maccmsEpisodesFromDocument(document)
	if len(episodes) != 5 {
		t.Fatalf("module-play-list-content episodes = %d, want 5 (2+3): %+v", len(episodes), episodes)
	}
	collapsed := maccmsCollapseRouteEpisodes(episodes)
	// 折叠后取集数最全线路（资源2，3 集）。
	if len(collapsed) != 3 {
		t.Fatalf("collapsed episodes = %d, want 3: %+v", len(collapsed), collapsed)
	}
	for index, episode := range collapsed {
		want := fmt.Sprintf("/vodplay/31722-2-%d.html", index+1)
		if episode.URL != want {
			t.Fatalf("collapsed episode %d = %q, want %q", index, episode.URL, want)
		}
		if episode.Title != fmt.Sprintf("第%d集", index+1) {
			t.Fatalf("collapsed title %d = %q", index, episode.Title)
		}
	}
}
