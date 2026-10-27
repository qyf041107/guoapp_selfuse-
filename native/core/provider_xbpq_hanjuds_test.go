package core

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
)

// 用户实际导入的规则（hanjuds 站点，macCMS10 默认模板）：列表条目为
// <div class="module-item">，数组用「同串首尾」写法切分。此前该源在本地服务版
// 一律报「规则未抽取到列表条目」——真实原因是自定义源 host 被登记进浏览器指纹
// 传输白名单后，站点对 utls 指纹返回 403 Cloudflare 验证页，而抓取错误被吞掉了。
const xbpqHanjudsRuleJSON = `{
"主页url": "https://www.hanjuds.com",
"请求头": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
"分类": "成全电影$1#成全剧集$2",
"分类url": "https://www.hanjuds.com/80s/index-{cateId}-{catePg}.html",
"搜索url": "https://www.hanjuds.com/80s/list--------------.html?wd={wd}",
"详情url": "https://www.hanjuds.com/80s/{id}.html",
"数组": "<div class=\"module-item\"&&<div class=\"module-item\"",
"标题": "title=\"&&\"",
"列表图片": "data-src=\"&&\"",
"副标题": "class=\"module-item-text\">&&</div>",
"链接": "href=\"&&\"",
"搜索数组": "<div class=\"module-search-item\"&&<div class=\"video-info-main\">",
"搜索标题": "<h3>&&</h3>",
"搜索链接": "video-serial\" href=\"&&\"",
"搜索图片": "data-src=\"&&\"",
"播放数组": "class=\"module-list module-player-list tab-list sort-list&&</div>",
"播放列表": "</a>",
"播放标题": "<span>&&</span>",
"播放链接": "href=\"&&\""
}`

func TestXBPQHanjudsRuleExtractsCatalog(t *testing.T) {
	body, err := os.ReadFile("testdata/xbpq_hanjuds_list.html")
	if err != nil {
		t.Fatalf("读取样本失败：%v", err)
	}
	d := sourceFixtureDownloader(t, func(request *http.Request) (*http.Response, error) {
		if !strings.Contains(request.URL.Path, "/80s/index-1-1.html") {
			t.Errorf("分类页地址错误：%s", request.URL)
		}
		return sourceFixtureResponse(request, 200, string(body)), nil
	})
	rule, ok := parseXBPQRule(xbpqHanjudsRuleJSON)
	if !ok {
		t.Fatal("规则解析失败")
	}
	items, hasMore, err := d.xbpqCatalog(context.Background(), "custom:hanjuds", "https://www.hanjuds.com", 1, "1", rule)
	if err != nil {
		t.Fatalf("目录抽取失败：%v", err)
	}
	if len(items) < 50 {
		t.Fatalf("样本页应抽到 50 条以上，实际 %d", len(items))
	}
	if !hasMore {
		t.Fatal("首屏应标记还有下一页")
	}
	first := items[0]
	if first.Title != "一生所爱" || first.SourceID != "157525" {
		t.Fatalf("首条解析错误：title=%q id=%q", first.Title, first.SourceID)
	}
	if cover, _ := first.Cover.(string); !strings.Contains(cover, "img.bshyw.com") {
		t.Fatalf("封面应取 列表图片（data-src），实际 %v", first.Cover)
	}
}

func TestXBPQCatalogSurfacesFetchFailure(t *testing.T) {
	d := sourceFixtureDownloader(t, func(request *http.Request) (*http.Response, error) {
		return sourceFixtureResponse(request, 403, "<html><title>Just a moment...</title></html>"), nil
	})
	rule, _ := parseXBPQRule(xbpqHanjudsRuleJSON)
	_, _, err := d.xbpqCatalog(context.Background(), "custom:hanjuds", "https://www.hanjuds.com", 1, "1", rule)
	if err == nil {
		t.Fatal("403 应当报错")
	}
	message := err.Error()
	if strings.Contains(message, "未抽取到列表条目") {
		t.Fatalf("抓取失败不该伪装成规则问题：%s", message)
	}
	if !strings.Contains(message, "抓取失败") {
		t.Fatalf("错误信息应说明是抓取失败：%s", message)
	}
}

func TestXBPQCatalogReportsArrayMismatch(t *testing.T) {
	d := sourceFixtureDownloader(t, func(request *http.Request) (*http.Response, error) {
		return sourceFixtureResponse(request, 200, "<html><body><p>站点维护中</p></body></html>"), nil
	})
	rule, _ := parseXBPQRule(xbpqHanjudsRuleJSON)
	_, _, err := d.xbpqCatalog(context.Background(), "custom:hanjuds", "https://www.hanjuds.com", 1, "1", rule)
	if err == nil {
		t.Fatal("空页面应当报错")
	}
	if !strings.Contains(err.Error(), "没有匹配到站点结构") {
		t.Fatalf("应提示数组模式与站点结构不匹配：%s", err.Error())
	}
}

// 该规则的「播放列表」用 </a> 作分集分隔符（整串字面切分），
// 「播放数组」按 module-player-list 容器截取；两者配合才能切出每一集。
func TestXBPQHanjudsRuleParsesEpisodes(t *testing.T) {
	page := `<div class="module-list module-player-list tab-list sort-list module-vod-list" id="glist-1">` +
		`<div class="module-blocklist"><div class="scroll-content scroll-y scroll-play">` +
		`<a href="/80s/play-156938-1-1.html" title="播放人生复本 第二季第01集"><span>第01集</span></a>` +
		`<a href="/80s/play-156938-1-2.html" title="播放人生复本 第二季第02集"><span>第02集</span></a>` +
		`<a href="/80s/play-156938-1-3.html" title="播放人生复本 第二季第03集"><span>第03集</span></a>` +
		`</div></div></div>`
	rule, _ := parseXBPQRule(xbpqHanjudsRuleJSON)
	episodes, routes := xbpqEpisodes(page, "https://www.hanjuds.com/80s/156938.html", rule)
	if len(episodes) != 3 {
		t.Fatalf("应切出 3 集，实际 %d（routes=%d）", len(episodes), len(routes))
	}
	if episodes[0].title != "第01集" || !strings.HasSuffix(episodes[0].url, "/80s/play-156938-1-1.html") {
		t.Fatalf("首集解析错误：title=%q url=%q", episodes[0].title, episodes[0].url)
	}
}
