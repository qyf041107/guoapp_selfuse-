package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// MacCMS 标准接口返回体（节选，字段名与线上一致）。
const maccmsAPISample = `{"code":1,"msg":"数据列表","page":"1","pagecount":11026,"limit":"20","total":220517,"list":[
{"vod_id":261911,"type_id":27,"type_name":"日韩动漫","vod_name":"大叔喜欢可爱小玩意",
 "vod_pic":"https:\/\/pic.example\/a.jpg","vod_remarks":"更新至第01集","vod_class":"喜剧,动画",
 "vod_content":"<p>小路三贵长相帅气<\/p>",
 "vod_play_from":"ffm3u8$$$bfzym3u8$$$rym3u8",
 "vod_play_url":"第01集$https://a.example/index.m3u8#第02集$https://a.example/2.m3u8$$$第01集$https://b.example/index.m3u8$$$第01集$https://c.example/index.m3u8"}
]}`

func TestMaccmsAPIItemsAndClasses(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(maccmsAPISample), &payload); err != nil {
		t.Fatalf("解包失败: %v", err)
	}
	items := maccmsAPIItems(payload)
	if len(items) != 1 {
		t.Fatalf("期望 1 条，实得 %d", len(items))
	}
	if maccmsAPIString(items[0]["vod_id"]) != "261911" {
		t.Fatalf("vod_id 解析错误: %v", items[0]["vod_id"])
	}
	if got := maccmsAPIInt(payload["pagecount"]); got != 11026 {
		t.Fatalf("pagecount 解析错误: %d", got)
	}
	var withClass map[string]any
	body := `{"code":1,"class":[{"type_id":1,"type_name":"电影"},{"type_id":2,"type_name":"连续剧"}]}`
	if err := json.Unmarshal([]byte(body), &withClass); err != nil {
		t.Fatalf("解包失败: %v", err)
	}
	if rows := maccmsAPIClasses(withClass); len(rows) != 2 {
		t.Fatalf("class 解析错误: %d", len(rows))
	}
}

func TestMaccmsAPIDramaAndChapters(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(maccmsAPISample), &payload); err != nil {
		t.Fatalf("解包失败: %v", err)
	}
	item := maccmsAPIItems(payload)[0]
	drama, chapters := maccmsAPIDrama("custom:test", "https://feikuai.in", item)
	if drama.ID != "custom:test:261911" {
		t.Fatalf("drama id 错误: %s", drama.ID)
	}
	if drama.Title != "大叔喜欢可爱小玩意" || drama.Category != "日韩动漫" {
		t.Fatalf("标题/分类错误: %q / %q", drama.Title, drama.Category)
	}
	if drama.Cover != "https://pic.example/a.jpg" {
		t.Fatalf("封面错误: %q", drama.Cover)
	}
	if drama.Intro != "小路三贵长相帅气" {
		t.Fatalf("简介未清洗标签: %q", drama.Intro)
	}
	// 三条线路分别 2 / 1 / 1 集，应取集数最全的第一条。
	if len(chapters) != 2 {
		t.Fatalf("应取 2 集，实得 %d", len(chapters))
	}
	if chapters[0].VideoURL != "https://a.example/index.m3u8" {
		t.Fatalf("分集地址错误: %q", chapters[0].VideoURL)
	}
	if chapters[0].Title != "第01集" {
		t.Fatalf("分集标题错误: %q", chapters[0].Title)
	}
	if number, ok := drama.EpisodeCount.(json.Number); !ok || number.String() != "2" {
		t.Fatalf("集数字段错误: %v", drama.EpisodeCount)
	}
}

func TestMaccmsAPIRouteGroupsFallback(t *testing.T) {
	// 老模板用换行分隔分集。
	item := map[string]any{
		"vod_play_from": "m3u8",
		"vod_play_url":  "第1集$https://x/1.m3u8\r\n第2集$https://x/2.m3u8\r\n第3集$https://x/3.m3u8",
	}
	routes := maccmsAPIRouteGroups(item)
	if len(routes) != 1 || len(routes[0]) != 3 {
		t.Fatalf("换行分隔解析失败: %+v", routes)
	}
}

const maccmsAPISampleXML = `<?xml version="1.0" encoding="utf-8"?>
<total>99</total>
<pagecount>5</pagecount>
<list pic_domain="http://img.example.com">
<video>
<id>1</id>
<name>测试剧</name>
<type>电影</type>
<note>更新至第2集</note>
<des>一部测试影片</des>
<vod_pic>/a.jpg</vod_pic>
<vod_play_url>第1集$https://v.example/1.m3u8#第2集$https://v.example/2.m3u8</vod_play_url>
</video>
<video>
<id>2</id>
<name>第二部</name>
<type>连续剧</type>
<vod_pic>/b.jpg</vod_pic>
<vod_play_url>第1集$https://v.example/c1.m3u8</vod_play_url>
</video>
</list>
<class>
<ty id="1">电影</ty>
<ty id="2">连续剧</ty>
</class>`

func TestMaccmsAPIParseXML(t *testing.T) {
	payload, err := maccmsAPIParseXML(maccmsAPISampleXML)
	if err != nil {
		t.Fatalf("XML 解析失败: %v", err)
	}
	items := maccmsAPIItems(payload)
	if len(items) != 2 {
		t.Fatalf("期望 2 条视频，实得 %d", len(items))
	}
	if maccmsAPIString(items[0]["vod_name"]) != "测试剧" {
		t.Fatalf("标题解析错误: %v", items[0]["vod_name"])
	}
	// 相对图片路径按 pic_domain 补全为绝对地址。
	if got := maccmsAPIString(items[0]["vod_pic"]); got != "http://img.example.com/a.jpg" {
		t.Fatalf("图片域补全错误: %q", got)
	}
	if got := maccmsAPIString(items[0]["type_name"]); got != "电影" {
		t.Fatalf("分类名解析错误: %q", got)
	}
	if maccmsAPIString(items[0]["vod_play_url"]) == "" {
		t.Fatal("分集地址未解析")
	}
	if rows := maccmsAPIClasses(payload); len(rows) != 2 {
		t.Fatalf("class 解析错误: %d", len(rows))
	}
	if got := maccmsAPIInt(payload["pagecount"]); got != 5 {
		t.Fatalf("pagecount 解析错误: %d", got)
	}
}

func TestMaccmsAPIParseXMLVideoToMapDLFallback(t *testing.T) {
	// 无 vod_play_url 时用 <dl><dd> 各集拼接（对齐 streamhub 的 dd 兜底）。
	body := `<video><id>9</id><name>拼接源</name><dl><dd>第1集$http://a/1.mp4</dd><dd>第2集$http://a/2.mp4</dd></dl></video>`
	payload, err := maccmsAPIParseXML(body)
	if err != nil {
		t.Fatalf("XML 解析失败: %v", err)
	}
	items := maccmsAPIItems(payload)
	if len(items) != 1 {
		t.Fatalf("期望 1 条，实得 %d", len(items))
	}
	playURL := maccmsAPIString(items[0]["vod_play_url"])
	if playURL != "第1集$http://a/1.mp4$$$第2集$http://a/2.mp4" {
		t.Fatalf("dd 拼接错误: %q", playURL)
	}
}

func TestMaccmsAPIParseXMLRejectsUnrelatedXML(t *testing.T) {
	body := `<?xml version="1.0"?><rss><channel><title>博客</title></channel></rss>`
	if _, err := maccmsAPIParseXML(body); err == nil {
		t.Fatal("非 MacCMS XML 应当报错")
	}
}

func TestMaccmsAPIBestRouteIndexPrefersStreamhubOrder(t *testing.T) {
	// 1) m3u8 优先于分集更多的 mp4 线路。
	routes := [][]maccmsAPIEpisode{
		{{Title: "1", URL: "https://x/a.mp4"}, {Title: "2", URL: "https://x/b.mp4"}},
		{{Title: "1", URL: "https://y/a.m3u8"}},
	}
	if got := maccmsAPIBestRouteIndex(routes); got != 1 {
		t.Fatalf("应选 m3u8 线路(1)，实得 %d", got)
	}
	// 2) 无 m3u8 时 mp4 优先取第一条含 mp4 的线路。
	routes2 := [][]maccmsAPIEpisode{
		{{Title: "1", URL: "https://x/a.mp4"}, {Title: "2", URL: "https://x/b.mp4"}},
		{{Title: "1", URL: "https://y/a.mp4"}},
	}
	if got := maccmsAPIBestRouteIndex(routes2); got != 0 {
		t.Fatalf("无 m3u8 应选第一条 mp4 线路(0)，实得 %d", got)
	}
	// 3) 都没有 m3u8/mp4 时退回分集最多。
	routes3 := [][]maccmsAPIEpisode{
		{{Title: "1", URL: "https://x/a.flv"}},
		{{Title: "1", URL: "https://y/a.flv"}, {Title: "2", URL: "https://y/b.flv"}, {Title: "3", URL: "https://y/c.flv"}},
	}
	if got := maccmsAPIBestRouteIndex(routes3); got != 1 {
		t.Fatalf("应选分集最多线路(1)，实得 %d", got)
	}
}

func TestMaccmsAPIEndpointCandidatesPreservesFullEndpoint(t *testing.T) {
	// 完整采集端点：base 本身排最前，且不再因已含 /at/ 而追加重复 at 变体。
	full := maccmsAPIEndpointCandidates("https://caiji.example.com/api.php/provide/vod/at/xml")
	if len(full) == 0 || full[0] != "https://caiji.example.com/api.php/provide/vod/at/xml" {
		t.Fatalf("完整端点应排最前: %v", full)
	}
	for _, candidate := range full {
		if candidate == "https://caiji.example.com/api.php/provide/vod/at/xml/at/json" {
			t.Fatalf("含 /at/ 的端点不应再追加 at 变体: %v", full)
		}
	}
	// 纯站点根：走通用候选，且补出 at/json 与 at/xml 变体。
	root := maccmsAPIEndpointCandidates("https://example.com")
	joined := ""
	for _, candidate := range root {
		joined += candidate + "\n"
	}
	if !strings.Contains(joined, "at/json") || !strings.Contains(joined, "at/xml") {
		t.Fatalf("站点根候选应含 at/json 与 at/xml 变体: %s", joined)
	}
}
