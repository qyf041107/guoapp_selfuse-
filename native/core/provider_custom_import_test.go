package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newCustomTestEngine(t *testing.T) *nativeEngine {
	t.Helper()
	return &nativeEngine{customRegistry: newCustomMaccmsRegistry(t.TempDir())}
}

func newCustomTestEngineWithDownloader(t *testing.T, client *http.Client) *nativeEngine {
	t.Helper()
	return &nativeEngine{
		customRegistry: newCustomMaccmsRegistry(t.TempDir()),
		downloader: &Downloader{
			cfg:           Config{dataDir: t.TempDir(), Retries: 1},
			client:        client,
			limiter:       newRequestLimiter(3, 0),
			providerHosts: map[string]string{},
		},
	}
}

func TestCustomSourceImportTVBoxMessyJSON(t *testing.T) {
	engine := newCustomTestEngine(t)
	// 真实 TVBox 配置常见形态：行注释、尾逗号、字段值里裸制表符/换行
	content := `{
	// 注释一
	"sites": [
		{
			"key": "ok",
			"name": "站点甲",
			"type": 1,
			"api": "https://a.example.com/api.php/provide/vod/",
			"note": "带	制表符	的值",
		},
		{
			"key": "tab",
			"name": "站点乙",   "type": 1,
			"api": "https://b.example.com/api.php/provide/vod/",
			"searchable": 1,
		},
		{
			"key": "esc",
			"name": "站点丙",
			"type": 1,
			"api": "https://c.example.com/api.php/provide/vod/",
			"note": "class=\'video\'&&</ul>",
		},
	],
}`
	result, err := engine.importCustomMaccmsSources(content)
	if err != nil {
		t.Fatalf(" messy TVBox 导入失败: %v", err)
	}
	if len(result.Added) != 3 {
		t.Fatalf("应导入 3 站，实际 %+v", result.Added)
	}
	if result.Added[0].Base != "https://a.example.com/api.php/provide/vod" {
		t.Fatalf("采集端点应保留完整 URL: %s", result.Added[0].Base)
	}
	if result.Added[2].Base != "https://c.example.com/api.php/provide/vod" {
		t.Fatalf("含非法转义 \\' 的条目应导入: %s", result.Added[2].Base)
	}
}

func TestCustomSourceRemoteURLImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/config/app.json" {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"sites":[{"key":"a","name":"远程站","type":1,"url":"https://remote.example.com"}]}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	engine := newCustomTestEngineWithDownloader(t, server.Client())
	// 带 .json 后缀的远程地址：应抓取后解析
	result, err := engine.importCustomMaccmsSources(server.URL + "/config/app.json")
	if err != nil {
		t.Fatalf("远程导入失败: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0].Name != "远程站" {
		t.Fatalf("远程导入结果错误: %+v", result)
	}
	// 站点根地址不触发抓取（当单站导入处理）
	result2, err := engine.importCustomMaccmsSources("https://plain.example.com")
	if err != nil {
		t.Fatalf("站点根地址导入失败: %v", err)
	}
	if len(result2.Added) != 1 || result2.Added[0].Base != "https://plain.example.com" {
		t.Fatalf("站点根地址应按单站导入: %+v", result2.Added)
	}
	// 无法访问的远程地址应报错而不是静默
	if _, err := engine.importCustomMaccmsSources(server.URL + "/missing.json"); err == nil {
		t.Fatal("404 远程地址应报错")
	}
}

func TestCustomSourceResetRestoresBuiltInOnly(t *testing.T) {
	engine := newCustomTestEngine(t)
	if _, err := engine.addCustomMaccmsSource("站一", "https://one.example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.addCustomMaccmsSource("站二", "https://two.example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	removed, err := engine.resetCustomMaccmsSources()
	if err != nil {
		t.Fatalf("恢复默认失败: %v", err)
	}
	if removed != 2 {
		t.Fatalf("应移除 2 个: %d", removed)
	}
	if len(engine.customMaccmsSources()) != 0 {
		t.Fatal("恢复默认后仍有残留")
	}
	// 持久化文件也应清空：重载后依然为空
	reloaded := newCustomMaccmsRegistry("")
	reloaded.path = engine.customRegistry.path
	reloaded.load()
	if len(reloaded.all()) != 0 {
		t.Fatal("恢复默认未落盘")
	}
	// 恢复后可以继续正常添加
	if _, err := engine.addCustomMaccmsSource("站三", "https://three.example.com", "", ""); err != nil {
		t.Fatalf("恢复后添加失败: %v", err)
	}
}

func TestCustomSourceExportImportRoundTrip(t *testing.T) {
	engine := newCustomTestEngine(t)
	if _, err := engine.addCustomMaccmsSource("我的影院", "https://my.example.com", "", ""); err != nil {
		t.Fatalf("添加失败: %v", err)
	}
	if _, err := engine.addCustomMaccmsSource("", "", xbpqCutRuleJSON, ""); err != nil {
		t.Fatalf("规则源添加失败: %v", err)
	}
	content, err := engine.exportCustomMaccmsSources()
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	var envelope struct {
		Format string `json:"format"`
		Sites  []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
			Rule any    `json:"rule"`
		} `json:"sites"`
	}
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		t.Fatalf("导出内容不是 JSON: %v", err)
	}
	if envelope.Format != customSourceExportFormat || len(envelope.Sites) != 2 {
		t.Fatalf("导出结构错误: %+v", envelope)
	}
	// 规则应以嵌套 JSON 对象形式随行（不再整段转义成字符串满屏 \"）
	var ruleSite *struct {
		Name string `json:"name"`
		URL  string `json:"url"`
		Rule any    `json:"rule"`
	}
	for index := range envelope.Sites {
		if envelope.Sites[index].Rule != nil && envelope.Sites[index].Rule != "" {
			ruleSite = &envelope.Sites[index]
		}
	}
	if ruleSite == nil {
		t.Fatal("导出未包含 XBPQ 规则")
	}
	if ruleObject, isObject := ruleSite.Rule.(map[string]any); !isObject {
		t.Fatalf("规则应以嵌套对象导出（去转义），实际 %T: %v", ruleSite.Rule, ruleSite.Rule)
	} else if ruleObject["主页url"] == nil {
		t.Fatalf("导出的规则对象缺少「主页url」: %v", ruleObject)
	}
	// 内容里不应再出现整段被转义的 \"主页url\" 字符串形态
	if strings.Contains(content, `\"主页url\"`) {
		t.Fatal("导出仍含转义后的规则字符串，未去转义")
	}
	// 新引擎导入同一份导出文件
	fresh := newCustomTestEngine(t)
	result, err := fresh.importCustomMaccmsSources(content)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(result.Added) != 2 || len(result.Failed) != 0 {
		t.Fatalf("导入结果错误: added=%d failed=%v skipped=%v", len(result.Added), result.Failed, result.Skipped)
	}
	for _, record := range result.Added {
		if strings.Contains(record.ID, " ") {
			t.Fatalf("导入条目 ID 非法: %+v", record)
		}
	}
	// 二次导入：同网址同规则 → 走更新而不是报错重复
	again, err := fresh.importCustomMaccmsSources(content)
	if err != nil {
		t.Fatalf("二次导入失败: %v", err)
	}
	if len(again.Updated) != 2 || len(again.Added) != 0 || len(again.Failed) != 0 {
		t.Fatalf("二次导入应全部更新: %+v", again)
	}
}

func TestCustomSourceImportPlainArrayAndTxt(t *testing.T) {
	engine := newCustomTestEngine(t)
	// 纯数组（字符串行）
	result, err := engine.importCustomMaccmsSources(`["https://a.example.com", "https://b.example.com"]`)
	if err != nil {
		t.Fatalf("数组导入失败: %v", err)
	}
	if len(result.Added) != 2 {
		t.Fatalf("数组导入数量错误: %+v", result)
	}
	// 纯文本每行一个，带名称前缀
	result2, err := engine.importCustomMaccmsSources("C站,https://c.example.com\nhttps://d.example.com\n# 注释行\ngarbage line here")
	if err != nil {
		t.Fatalf("文本导入失败: %v", err)
	}
	if len(result2.Added) != 2 {
		t.Fatalf("文本导入数量错误: %+v", result2)
	}
	names := map[string]bool{}
	for _, record := range result2.Added {
		names[record.Name] = true
	}
	if !names["C站"] {
		t.Fatalf("名称前缀未解析: %v", names)
	}
}

func TestCustomSourceImportTVBoxSites(t *testing.T) {
	engine := newCustomTestEngine(t)
	content := `{
	  "sites": [
		{"key": "cntv", "name": "💕推荐采集", "type": 1, "api": "https://collect.example.com/api.php/provide/vod/"},
		{"key": "sp", "name": "爬虫站", "type": 3, "jar": "csp_XBPQ", "api": "https://sp.example.com"},
		{"key": "xbpq", "name": "规则站", "type": 1, "api": "https://x.example.com", "ext": {"主页url": "https://x.example.com", "分类url": "https://x.example.com/{cateId}-{catePg}.html", "分类": "电影$1#电视剧$2", "数组": "<a&&</a>", "标题": "title=\"&&\"", "链接": "href=\"&&\""}}
	  ]
	}`
	result, err := engine.importCustomMaccmsSources(content)
	if err != nil {
		t.Fatalf("TVBox 导入失败: %v", err)
	}
	if len(result.Added) != 2 {
		t.Fatalf("期望 2 条成功: %+v", result)
	}
	if len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0], "jar") {
		t.Fatalf("爬虫条目应跳过: %+v", result.Skipped)
	}
	var ruleRecord *customMaccmsSource
	for index := range result.Added {
		if result.Added[index].Rule != "" {
			ruleRecord = &result.Added[index]
		}
	}
	if ruleRecord == nil {
		t.Fatal("ext 规则未识别")
	}
	if ruleRecord.Base != "https://x.example.com" {
		t.Fatalf("规则源 base 错误: %q", ruleRecord.Base)
	}
	// 采集站保留完整端点
	var apiRecord *customMaccmsSource
	for index := range result.Added {
		if result.Added[index].Rule == "" {
			apiRecord = &result.Added[index]
		}
	}
	if apiRecord == nil || apiRecord.Base != "https://collect.example.com/api.php/provide/vod" {
		t.Fatalf("api 地址应保留完整端点: %+v", result.Added)
	}
}

func TestCustomSourceImportGarbageFails(t *testing.T) {
	engine := newCustomTestEngine(t)
	if _, err := engine.importCustomMaccmsSources("hello world 不是站点"); err == nil {
		t.Fatal("垃圾内容应整体失败")
	}
	if _, err := engine.importCustomMaccmsSources(""); err == nil {
		t.Fatal("空内容应失败")
	}
}

func TestCustomSourceSplicedAcceleratorPreserved(t *testing.T) {
	engine := newCustomTestEngine(t)
	// 加速链接+网址拼接形态：必须作为整体保留，不能只取斜杠前的加速域名。
	spliced := "https://cfkua.wokaotianshi.eu.org/https://caiji.maotaizy.cc"
	record, err := engine.addCustomMaccmsSource("拼接源", spliced, "", "")
	if err != nil {
		t.Fatalf("拼接地址添加失败: %v", err)
	}
	if record.Base != spliced {
		t.Fatalf("拼接地址应整体保留，实际 %q", record.Base)
	}
	// 导入路径同样要识别并保留。
	fresh := newCustomTestEngine(t)
	result, err := fresh.importCustomMaccmsSources(
		`{"name":"拼接站","url":"https://cfkua.wokaotianshi.eu.org/https://caiji.maotaizy.cc"}`)
	if err != nil {
		t.Fatalf("拼接地址导入失败: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0].Base != spliced {
		t.Fatalf("导入未整体保留拼接地址: %+v", result.Added)
	}
	// 普通 maccms api 地址现保留完整采集端点（与新行为一致），且不误判为拼接。
	result2, err := fresh.importCustomMaccmsSources(
		`{"name":"api站","api":"https://plain.example.com/api.php/provide/vod/"}`)
	if err != nil {
		t.Fatalf("api 地址导入失败: %v", err)
	}
	if len(result2.Added) != 1 || result2.Added[0].Base != "https://plain.example.com/api.php/provide/vod" {
		t.Fatalf("api 地址应保留完整端点: %+v", result2.Added)
	}
}

func TestCustomSourceNoCountLimit(t *testing.T) {
	engine := newCustomTestEngine(t)
	// 32 上限已移除：一次导入 40 站应全部成功，不再有「超出上限」的跳过。
	lines := make([]string, 0, 40)
	for index := 0; index < 40; index++ {
		lines = append(lines, fmt.Sprintf("https://s%02d.example.com", index))
	}
	result, err := engine.importCustomMaccmsSources(strings.Join(lines, "\n"))
	if err != nil {
		t.Fatalf("批量导入失败: %v", err)
	}
	if len(result.Added) != 40 {
		t.Fatalf("40 站应全部导入，实际 %d: skipped=%v failed=%v",
			len(result.Added), result.Skipped, result.Failed)
	}
	if len(engine.customMaccmsSources()) != 40 {
		t.Fatalf("注册表应有 40 个源，实际 %d", len(engine.customMaccmsSources()))
	}
}

func TestCustomSourceExportNestedRuleNoEscapes(t *testing.T) {
	engine := newCustomTestEngine(t)
	if _, err := engine.addCustomMaccmsSource("规则站", "", xbpqCutRuleJSON, ""); err != nil {
		t.Fatalf("添加规则源失败: %v", err)
	}
	content, err := engine.exportCustomMaccmsSources()
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	// 规则以嵌套对象随行，正文里不应再出现整段被转义的 \"主页url\"。
	if strings.Contains(content, `\"主页url\"`) {
		t.Fatalf("导出规则仍带转义，未还原成对象: %s", content)
	}
	// round-trip：新引擎导入自己导出的内容，规则应命中「更新」而非「已存在跳过」。
	fresh := newCustomTestEngine(t)
	if _, err := fresh.importCustomMaccmsSources(content); err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	again, err := fresh.importCustomMaccmsSources(content)
	if err != nil {
		t.Fatalf("二次导入失败: %v", err)
	}
	if len(again.Updated) != 1 || len(again.Added) != 0 {
		t.Fatalf("二次导入应更新而非新增: %+v", again)
	}
}
