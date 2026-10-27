package core

// ---- XBPQ 内置模板（对齐官方 XBPQ.jar 的「简写」能力）----
//
// 实测依据（XBPQ.json 334 份 csp_XBPQ 源全量统计）：
//   - 203/334（61%）不写「主页url」，主页由 分类url 的站点根推导；
//   - 185/334（55%）只写「分类url+分类」即正常工作——jar 按 分类url 形态
//     自动套用内置模板（数组/标题/链接/图片/播放/搜索全套默认）。
// 本文件为 provider_xbpq.go 补上同一能力：识别 MacCMS/stui 两大模板家族，
// 为缺失字段补 || 多组备选的默认截取 pattern，已写字段一律不覆盖。
//
// 与 wrxbpq（写源工具）的 internal/xbpq/template.go 保持逐项同步：
// 家族名、match 正则、字段值都以那边为准，那边实测过新皮肤后再同步到这里。
// 最近一轮同步点：
//   - MacCMS 家族 match 放宽到 /index.php/(vod|art)/、/vodplay|voddetail/、
//     /list/{cateId}-…html 伪静态、vod-play-id- 短路径；
//   - MacCMS 数组首个备选补 `<a ` 前缀（myui 的 <a> 由 class 起头）；
//   - 路径式家族 match 补 type|category|fenlei 并去掉「cateId 可省略」的松匹配，
//     播放数组补 myui / content-list 容器；
//   - 路径式 播放列表 由（错误的）截取串 `<a&&</a>||<li&&</li>` 改为分隔符 `<li`
//     ——播放列表是分集分隔符，写整串永远切不中，整段会塌成 1 集。

import (
	"regexp"
	"strings"
)

// xbpqTemplateFamily 一种 分类url 形态对应的默认字段集。
// 家族按「专用 → 通用」排序，命中的家族**依次合并**（先命中字段优先，
// 后面的家族只补还没被覆盖的字段）——与 wrxbpq 的 template.go 同步。
type xbpqTemplateFamily struct {
	name   string
	match  *regexp.Regexp
	fields map[string]string
}

var xbpqTemplateFamilies = []xbpqTemplateFamily{
	{
		// 苹果CMS新版 mxone 皮肤（/template/mxone/，如 hanjuds.com、555yx.vip 分集区）：
		// 分类形态「目录 + index-{cateId}-{catePg}.html」（列表目录名与域名无关，match 只认 index- 段）；
		// 详情 /{dir}/{数字}.html、播放 /{dir}/play-{id}-{线}-{集}.html、
		// 列表条目 div class="module-item"、多线路 div class="module-list module-player-list tab-list sort-list"。
		// 关键差异：
		//   ① 线路容器与分集容器同锚点——线路数组锚 tab-list（截出的每段=该线路分集框），
		//      锚 module-tab-item 按钮会让每条线路 0 集；
		//   ② 分集是 <a> 连排、段内无 #，播放列表分隔符写 </a>（不是 <li>/不是 #）；
		//   ③ 取流是 iframe+base64 二级页（src="…/url?url=<base64 m3u8>"），
		//      maccmsPlayerURL 末路解码，跳转播放链接【省略不写】。
		name:  "MacCMS新版mxone",
		match: regexp.MustCompile(`(?i)/index-(?:\{cateId\}|[0-9])[^/"?]*\.html`),
		fields: map[string]string{
			"数组":   `<div class="module-item"&&<div class="module-item"||class="module-item"><div class="module-item-cover"&&</div></div>||<div class="module-item"&&</div></div>||<li&&</li>`,
			"标题":   `title="&&"`,
			"副标题":  `class="module-item-text">&&</div>`,
			"图片":   `data-src="&&"||data-original="&&"`,
			"链接":   `href="&&"`,
			"播放数组": `class="module-list module-player-list tab-list sort-list&&</div>`,
			"线路数组": `class="module-list module-player-list tab-list sort-list&&</div>`,
			"播放列表": `</a>`,
			"播放标题": `<span>&&</span>`,
			"播放链接": `href="&&"`,
			"搜索数组": `<div class="module-search-item"&&<div class="video-info-main">`,
			"搜索标题": `<h3>&&</h3>`,
			"搜索链接": `video-serial" href="&&"`,
			"搜索图片": `data-src="&&"`,
		},
	},
	{
		// MacCMS 自带 API（/api.php/provide/vod/，?ac=videolist|list）：列表层是 JSON。
		// 模板库 §五：接口站列表用 j: 模式，详情/播放仍走 HTML 详情页。
		name:  "MacCMS接口(JSON)",
		match: regexp.MustCompile(`(?i)api\.php/provide/vod|/provide/vod/?|ac=videolist`),
		fields: map[string]string{
			"数组":     `j:list`,
			"标题":     `j:vod_name`,
			"图片":     `j:vod_pic`,
			"副标题":    `j:vod_remarks`,
			"链接":     `/index.php/vod/detail/id/+j:vod_id+.html`,
			"详情url":  `/index.php/vod/detail/id/{id}.html`,
			"搜索url":  `{host}/index.php/vod/search.html?wd={wd}`,
			"播放数组":   `<ul class="stui-content__playlist&&</ul>||<ul class="myui-content__list&&</ul>`,
			"播放列表":   `<a`,
			"播放标题":   `>&&</a>||>&&<`,
			"播放链接":   `href="&&"`,
			"跳转播放链接": `var player_*"url":"&&"`,
		},
	},
	{
		// 苹果CMS/MacCMS 家族：/index.php/vod/show|type|art/…、/vodshow/…、/vodtype/…、
		// /list/{cateId}-{catePg}.html 伪静态、vod-list-id-/vod-play-id- 短路径。
		// 页面多为 stui / myui 模板；详情/播放/搜索均有高置信默认。
		// || 备选链按 XBPQ.json 379 份真实规则聚类高频形态排序（myui→stui→hl→module→通用）。
		// mxone poster-items 皮肤（555yx.vip / qqys01.com 实测）：数组必须用
		// `<a &&</a>[包含:module-poster-item module-item]`——直接锚
		// `module-poster-item&&</a>` 会把卡片截半、目录塌成 1 条。
		name:  "MacCMS",
		match: regexp.MustCompile(`(?i)/index\.php/(vod|art)/|/vod(show|type|play|detail)/|/list/(?:\{cateId\}|[0-9])[^/"]*\.html|vod-(?:list|play)-id-`),
		fields: map[string]string{
			// 首个备选带 `<a ` 前缀（myui 的 <a class="myui-vodlist__thumb …"> 由 class 起头，
			// 缺了 `<a` 段内会丢开标签，个别皮肤下 链接 截不出来）。
			"数组":   `<a class="myui-vodlist__thumb&&</a>||class="myui-vodlist__box&&</a>||class="stui-vodlist__thumb&&</a>||stui-vodlist__box">&&</div></div>||hl-item-thumb hl-lazy"&&</a>||<a &&</a>[包含:module-poster-item module-item]||<li class="col-md-6&&</li>||<li&&</li>`,
			"标题":   `title="&&"||alt="&&"`,
			"图片":   `data-original="&&"||data-src="&&"||src="&&"`,
			"副标题":  `pic-text text-right">&&</span>||class="remark">&&<||module-item-text">&&</div>||class="module-item-note">&&</div>`,
			"链接":   `href="&&"`,
			"简介":   `detail-content" style=*>&&</span>||class="stui-content__detail&&</div>||简介：</em>&&||class="module-info-intro-content&&</div>`,
			"播放数组": `<ul class="stui-content__playlist&&</ul>||<ul class="myui-content__list&&</ul>||id="hl-plays-list"&&</ul>||mod play-list&&</ul||class="play_box&&</div>||class="module-play-list-content&&</div>`,
			// mxone 皮肤线路数组：每个 module-play-list-content 容器即一条线路，
			// 引擎按线路数组循环截取并以 $$$ 拼多线路（xbpqEpisodes）。
			"线路数组": `class="module-play-list-content&&</div>`,
			// 播放列表是**分集分隔符**（字面 Split），不是截取串：`<a` 切出每个
			// 分集片段后由 播放标题/播放链接 再截；写 `<a&&</a>` 会切不中塌成 1 集。
			"播放列表": `<a`,
			"播放标题": `>&&</a>||>&&<||<span>&&</span>`,
			"播放链接": `href="&&"`,
			"线路标题": `<h3 class="title">&&</h3>||<span class="title">&&</span>||play_tit&&</||data-name="&&"`,
			// 聚类最高频跳转形态（15 处，含尾引号——单侧无尾锚会吃到页尾）：
			// var player_ 前缀锚点不会误命中同页 var maccms 的 "url":"；
			// player_aaaa_ 是 mxone（苹果新版）皮肤的同族变体（qqys01 实测）。
			"跳转播放链接": `var player_*"url":"&&"||player_aaaa_*"url":"&&"`,
			"搜索url":  `{host}/index.php/vod/search.html?wd={wd}`,
			"搜索数组":   `<div class="module-card-item module-item&&<div class="module-card-item module-item||class="stui-vodlist__thumb&&</a>||<li&&</li>`,
			"搜索标题":   `title="&&"||<strong>&&</strong>||alt="&&"`,
			"搜索链接":   `href="&&"`,
			"搜索图片":   `data-original="&&"||data-src="&&"`,
		},
	},
	{
		// 短剧/路径式 CMS（模板库 §3/§4）：/show/{id}-{area}-…-{pg}---{year}.html、/vs/、
		// /vshow/、/type/{slug}/{pg}/、/category/…、/fenlei/… 等。
		// conch/module/stui 皮肤混杂，备选链兜底；这一族没有可靠通用默认，只给通用高频值。
		name:  "路径式泛型",
		match: regexp.MustCompile(`(?i)/(show|vs|vshow|screen|list|type|category|fenlei)/(?:\{cateId\}|[0-9a-z])`),
		fields: map[string]string{
			"数组":  `class="stui-vodlist__thumb&&</a>||module-item&&</a>||<li&&</li>`,
			"标题":  `title="&&"||alt="&&"`,
			"图片":  `data-original="&&"||data-src="&&"||src="&&"`,
			"副标题": `pic-text text-right">&&</span>||module-item-text">&&</div>`,
			"链接":  `href="&&"`,
			"简介":  `detail-content" style=*>&&</span>||class="video-info&&</div>`,
			// 容器备选顺序与分隔符 <li 对齐：先 <li> 连排的 ul（stui/myui/content-list），
			// 再 <a> 连排的 div（module-play-list，靠引擎的分隔符自适应兜底）。
			"播放数组": `<ul class="stui-content__playlist&&</ul>||<ul class="myui-content__list&&</ul>||<ul class="content-list&&</ul>||class="module-play-list&&</div>`,
			// 分隔符：<li> 连排写 <li（写整串 <li&&</li> 会切不中，整段塌成 1 集）。
			"播放列表":   `<li`,
			"播放标题":   `>&&</a>||>&&<`,
			"播放链接":   `href="&&"`,
			"线路标题":   `<h3 class="title">&&</h3>`,
			"跳转播放链接": `var player_*"url":"&&"`,
		},
	},
	{
		// 通用高频字段兜底（wrxbpq 模板库 §〇全局 Top-1）：任何有 分类url 的形态都命中。
		// 只补全局最稳的字段，绝不猜 数组/播放数组 这类站专属结构。
		name:  "通用高频",
		match: regexp.MustCompile(`.`),
		fields: map[string]string{
			"标题":   `title="&&"||alt="&&"`,
			"链接":   `href="&&"`,
			"图片":   `data-original="&&"||data-src="&&"||src="&&"`,
			"播放标题": `>&&</a>||>&&<`,
			"播放链接": `href="&&"`,
		},
	},
}

// xbpqApplyTemplate 按 分类url 形态补齐缺失字段（不覆盖已写字段）。幂等。
func (r *xbpqRule) xbpqApplyTemplate() {
	if r.fields == nil {
		r.fields = map[string]string{}
	}
	if r.fields["模板已套用"] != "" {
		return
	}
	category := r.field("分类url", "分类Url")
	if category == "" {
		return
	}
	for _, fam := range xbpqTemplateFamilies {
		if !fam.match.MatchString(category) {
			continue
		}
		origin := xbpqHostOrigin(category)
		for key, value := range fam.fields {
			key = xbpqNormalizeKey(key)
			if strings.Contains(value, "{host}") {
				if origin == "" {
					continue
				}
				value = strings.ReplaceAll(value, "{host}", origin)
			}
			if strings.TrimSpace(r.fields[key]) == "" {
				r.fields[key] = value
				r.order = append(r.order, key)
			}
		}
		// 不 break：与 wrxbpq template.go 同步——专用→通用依次合并，
		// 后面的命中家族只补还没被覆盖的字段（如 mxone 命中后仍要
		// MacCMS/通用高频 补 简介/搜索url/跳转播放链接 等缺口）。
	}
	r.fields["模板已套用"] = "1"
}

// xbpqHostOrigin 从绝对地址提取 scheme://host。
func xbpqHostOrigin(address string) string {
	index := strings.Index(address, "://")
	if index < 0 {
		return ""
	}
	rest := address[index+3:]
	if slash := strings.IndexAny(rest, "/?#"); slash >= 0 {
		rest = rest[:slash]
	}
	return address[:index+3] + rest
}

// xbpqEmptyPathSegment 匹配模板空值留下的路径段：/area//id → 整段删除。
// MacCMS 路径式 URL 对空筛选段返回 404；jar 的行为是自动省略空段。
var xbpqEmptyPathSegment = regexp.MustCompile(`/[a-zA-Z][a-zA-Z0-9_]*//`)

// xbpqEmptyTailSegment 匹配末段空值：/year/.html → .html。
var xbpqEmptyTailSegment = regexp.MustCompile(`/[a-zA-Z][a-zA-Z0-9_]*/(\.[a-zA-Z]{2,5}(?:\?|$))`)

// xbpqCleanEmptySegments 清洗 分类url 渲染后的空筛选段（含 query 空参数）。
func xbpqCleanEmptySegments(address string) string {
	for xbpqEmptyPathSegment.MatchString(address) {
		address = xbpqEmptyPathSegment.ReplaceAllString(address, "/")
	}
	for xbpqEmptyTailSegment.MatchString(address) {
		address = xbpqEmptyTailSegment.ReplaceAllString(address, "$1")
	}
	if queryIndex := strings.Index(address, "?"); queryIndex >= 0 {
		head, query := address[:queryIndex], address[queryIndex+1:]
		var kept []string
		dropped := false
		for _, pair := range strings.Split(query, "&") {
			if key, value, found := strings.Cut(pair, "="); found && value == "" && !strings.Contains(key, "{") {
				dropped = true
				continue
			}
			if pair != "" {
				kept = append(kept, pair)
			}
		}
		if dropped {
			if len(kept) == 0 {
				return head
			}
			return head + "?" + strings.Join(kept, "&")
		}
	}
	return address
}
