package core

import "testing"

func TestNativeSearchMergeSameDramaAcrossSources(t *testing.T) {
	merged := nativeSearchMerge([]nativeDrama{
		{ID: "hongguo:1", Source: "hongguo", Title: "夜色温柔", Episodes: 24, OnlineDate: "2025-03-01"},
		{ID: "yeguo:9", Source: "yeguo", Title: "夜色温柔", Episodes: 24, OnlineDate: "2025-03-01", Cover: "https://example.com/a.jpg"},
		{ID: "dsd:5", Source: "dsd", Title: "夜色温柔", Episodes: 24, OnlineDate: "2025-03-01"},
	})
	if len(merged) != 1 {
		t.Fatalf("期望归一为 1 条，实际 %d 条", len(merged))
	}
	// 带封面的条目信息更完整，应被选为主条目。
	if merged[0].Source != "yeguo" {
		t.Fatalf("期望主条目来自 yeguo，实际 %s", merged[0].Source)
	}
	if len(merged[0].Peers) != 2 {
		t.Fatalf("期望 2 个其它站源副本，实际 %d", len(merged[0].Peers))
	}
}

func TestNativeSearchMergeKeepsDifferentTitles(t *testing.T) {
	merged := nativeSearchMerge([]nativeDrama{
		{ID: "hongguo:1", Source: "hongguo", Title: "夜色温柔"},
		{ID: "yeguo:2", Source: "yeguo", Title: "夜色微凉"},
	})
	if len(merged) != 2 {
		t.Fatalf("标题不同不应合并，实际 %d 条", len(merged))
	}
}

func TestNativeSearchMergeKeepsDifferentYears(t *testing.T) {
	merged := nativeSearchMerge([]nativeDrama{
		{ID: "hongguo:1", Source: "hongguo", Title: "夜色温柔", OnlineDate: "2023-05-01"},
		{ID: "yeguo:2", Source: "yeguo", Title: "夜色温柔", OnlineDate: "2025-05-01"},
	})
	if len(merged) != 2 {
		t.Fatalf("年份不同不应合并，实际 %d 条", len(merged))
	}
}

func TestNativeSearchMergeKeepsFarApartEpisodeCounts(t *testing.T) {
	merged := nativeSearchMerge([]nativeDrama{
		{ID: "hongguo:1", Source: "hongguo", Title: "夜色温柔", Episodes: 12},
		{ID: "yeguo:2", Source: "yeguo", Title: "夜色温柔", Episodes: 80},
	})
	if len(merged) != 2 {
		t.Fatalf("集数差异过大不应合并，实际 %d 条", len(merged))
	}
}

func TestNativeSearchMergeToleratesMissingEpisodeCount(t *testing.T) {
	merged := nativeSearchMerge([]nativeDrama{
		{ID: "hongguo:1", Source: "hongguo", Title: "夜色温柔", Episodes: 0},
		{ID: "yeguo:2", Source: "yeguo", Title: "夜色温柔", Episodes: 80},
	})
	if len(merged) != 1 {
		t.Fatalf("集数缺失时应合并，实际 %d 条", len(merged))
	}
}

func TestNativeSearchMergeNormalizesPunctuation(t *testing.T) {
	merged := nativeSearchMerge([]nativeDrama{
		{ID: "hongguo:1", Source: "hongguo", Title: "夜色温柔：第一季"},
		{ID: "yeguo:2", Source: "yeguo", Title: "夜色温柔 第一季"},
	})
	if len(merged) != 1 {
		t.Fatalf("标点/空格差异应归一合并，实际 %d 条", len(merged))
	}
}

func TestNativeSearchSourceRejectsUnsupportedSource(t *testing.T) {
	if nativeSourceSupportsSearch(sourceHongguo) != true {
		t.Fatal("红果应支持在线搜索")
	}
	if nativeSourceSupportsSearch("definitely-not-a-source") {
		t.Fatal("未知站源不应判定为支持在线搜索")
	}
}
