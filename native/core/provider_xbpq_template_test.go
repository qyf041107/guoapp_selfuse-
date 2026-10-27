package core

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// 本文件固化「内置模板与 wrxbpq 写源工具同步」的关键点。
// wrxbpq 的 internal/xbpq/template.go 是模板的唯一真源：那边实测新皮肤后
// 更新 match / 字段链，再把同样的值同步到这里，两侧用这些用例对齐。

// xbpqTemplateFamiliesFor 返回某个 分类url 形态命中的家族名（按合并顺序）。
func xbpqTemplateFamiliesFor(category string) []string {
	var names []string
	for _, family := range xbpqTemplateFamilies {
		if family.match.MatchString(category) {
			names = append(names, family.name)
		}
	}
	return names
}

func xbpqHasFamily(names []string, wanted string) bool {
	for _, name := range names {
		if name == wanted {
			return true
		}
	}
	return false
}

func TestXBPQTemplateFamiliesMatchWrxbpq(t *testing.T) {
	cases := []struct {
		category string
		family   string
	}{
		// §8 苹果CMS新版 mxone（index-{cateId}-{catePg}.html）
		{"https://www.hanjuds.com/80s/index-{cateId}-{catePg}.html", "MacCMS新版mxone"},
		// §1 MacCMS：dash vodshow / index.php vod / art / 伪静态 list / play-detail 短路径
		{"https://a.com/vodshow/{cateId}--------{catePg}---.html", "MacCMS"},
		{"https://a.com/index.php/vod/show/id/{cateId}.html", "MacCMS"},
		{"https://a.com/index.php/art/show/id/{cateId}.html", "MacCMS"},
		{"https://a.com/vodplay/{cateId}-1-1.html", "MacCMS"},
		{"https://a.com/list/{cateId}-{catePg}.html", "MacCMS"},
		{"https://a.com/vod-list-id-{cateId}-pg-{catePg}.html", "MacCMS"},
		// §3/§4 路径式泛型：show/vs/vshow/screen/list/type/category/fenlei
		{"https://a.com/show/{cateId}-{area}----{catePg}---{year}.html", "路径式泛型"},
		{"https://a.com/vs/{cateId}-{catePg}.html", "路径式泛型"},
		{"https://a.com/type/{cateId}/{catePg}/", "路径式泛型"},
		{"https://a.com/category/{cateId}/{catePg}/", "路径式泛型"},
		{"https://a.com/fenlei/{cateId}/", "路径式泛型"},
	}
	for _, item := range cases {
		names := xbpqTemplateFamiliesFor(item.category)
		if !xbpqHasFamily(names, item.family) {
			t.Errorf("分类url %q 未命中家族 %s（实际命中 %v）", item.category, item.family, names)
		}
	}
	// 任何形态都应落到「通用高频」兜底，保证 标题/链接/图片 至少有默认值。
	if names := xbpqTemplateFamiliesFor("https://a.com/whatever"); !xbpqHasFamily(names, "通用高频") {
		t.Fatalf("未命中通用高频兜底: %v", names)
	}
}

func TestXBPQPathTemplatePlaylistIsSeparator(t *testing.T) {
	// 播放列表 是分集【分隔符】，写整串 `<a&&</a>||<li&&</li>` 永远切不中，
	// 整段会塌成 1 集（wrxbpq TestEpisodesListSplitIsSeparator 同一回归点）。
	rule, ok := parseXBPQRule(`{"主页url":"https://p.example.com","分类url":"https://p.example.com/type/{cateId}/{catePg}/","分类":"短剧$1"}`)
	if !ok {
		t.Fatal("路径式简写规则解析失败")
	}
	if split := rule.field("播放列表"); split != "<li" {
		t.Fatalf("路径式模板 播放列表 应为分隔符 <li，实际 %q", split)
	}
	for _, want := range []string{`<ul class="myui-content__list&&</ul>`, `<ul class="content-list&&</ul>`} {
		if !strings.Contains(rule.field("播放数组"), want) {
			t.Errorf("路径式模板 播放数组 缺少备选 %q", want)
		}
	}

	body := `<html><body><div class="play-box"><ul class="stui-content__playlist clearfix">` +
		`<li><a href="/p/1-1-1.html" title="第01集">第01集</a></li>` +
		`<li><a href="/p/1-1-2.html" title="第02集">第02集</a></li>` +
		`<li><a href="/p/1-1-3.html" title="第03集">第03集</a></li>` +
		`</ul></div></body></html>`
	episodes, _ := xbpqEpisodes(body, "https://p.example.com/p/1.html", rule)
	if len(episodes) != 3 {
		t.Fatalf("模板按 <li 切分应得 3 集，实际 %d: %+v", len(episodes), episodes)
	}
	if episodes[0].title != "第01集" || !strings.HasSuffix(episodes[2].url, "1-1-3.html") {
		t.Fatalf("分集解析错误: %+v", episodes)
	}
}

func TestXBPQPlaylistSeparatorFallsBackToAnchorChain(t *testing.T) {
	// 模板给的唯一分隔符与该站容器不符：容器是 <a> 连排（module-play-list），
	// 按 <li 只切出 1 段，引擎应识别「这段里有多个 href」并改用 <a 再切。
	rule, ok := parseXBPQRule(`{"主页url":"https://p.example.com","分类url":"https://p.example.com/type/{cateId}/{catePg}/","分类":"短剧$1"}`)
	if !ok {
		t.Fatal("路径式简写规则解析失败")
	}
	body := `<html><body><div class="module-play-list">` +
		`<a class="module-play-list-link" href="/p/2-1-1.html"><span>第1集</span></a>` +
		`<a class="module-play-list-link" href="/p/2-1-2.html"><span>第2集</span></a>` +
		`</div></body></html>`
	episodes, _ := xbpqEpisodes(body, "https://p.example.com/p/2.html", rule)
	if len(episodes) != 2 {
		t.Fatalf("分隔符自适应应切出 2 集，实际 %d: %+v", len(episodes), episodes)
	}
	if episodes[0].title != "第1集" || !strings.HasSuffix(episodes[1].url, "2-1-2.html") {
		t.Fatalf("分集解析错误: %+v", episodes)
	}
}

func TestXBPQPlaylistSeparatorKeepsSingleEpisode(t *testing.T) {
	// 反例保护：电影只有 1 个分集链接时，自适应兜底不能把 1 集拆成多集。
	rule, ok := parseXBPQRule(`{"主页url":"https://p.example.com","分类url":"https://p.example.com/type/{cateId}/{catePg}/","分类":"电影$1"}`)
	if !ok {
		t.Fatal("路径式简写规则解析失败")
	}
	body := `<html><body><div class="play-box"><ul class="stui-content__playlist">` +
		`<li><a href="/p/3-1-1.html">正片</a></li>` +
		`</ul></div></body></html>`
	episodes, _ := xbpqEpisodes(body, "https://p.example.com/p/3.html", rule)
	if len(episodes) != 1 {
		t.Fatalf("单片电影应保持 1 集，实际 %d: %+v", len(episodes), episodes)
	}
}

func TestXBPQListCoverPrefersListPicture(t *testing.T) {
	// 新写源工具把列表层封面写成 列表图片（图片/主图/封面图 是别名）。
	// 规则同时写了 列表图片 与 图片 时，目录封面必须取 列表图片。
	d, server := xbpqFixtureServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<html><body>` +
			`<a class="item" href="/voddetail/7.html" title="封面样本">` +
			`<img data-original="/pic/list7.jpg" src="/pic/placeholder.gif"></a>` +
			`</body></html>`))
	})
	rule, ok := parseXBPQRule(`{"主页url":"` + server.URL + `",` +
		`"分类url":"` + server.URL + `/vodshow/{cateId}--------{catePg}---.html",` +
		`"分类":"电影$1","数组":"<a class=\"item\"&&</a>","标题":"title=\"&&\"",` +
		`"链接":"href=\"&&\"","列表图片":"data-original=\"&&\"","图片":"src=\"&&\""}`)
	if !ok {
		t.Fatal("列表封面规则解析失败")
	}
	d.providerMu.Lock()
	d.providerHosts["custom:cvtest"] = server.URL
	d.providerMu.Unlock()
	items, _, err := d.xbpqCatalog(context.Background(), "custom:cvtest", server.URL, 1, "1", rule)
	if err != nil {
		t.Fatalf("目录抓取失败: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("期望 1 条，实际 %d: %+v", len(items), items)
	}
	if want := server.URL + "/pic/list7.jpg"; items[0].Cover != want {
		t.Fatalf("列表封面应取 列表图片（%s），实际 %q", want, items[0].Cover)
	}
}
