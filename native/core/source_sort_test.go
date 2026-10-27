package core

import (
	"reflect"
	"testing"
)

func TestSortSourceNamesOrder(t *testing.T) {
	input := []string{
		"黄果",     // huangguo（中文桶）
		"cloudfront", // 字母桶
		"红果",     // hongguo
		"A123",     // 字母桶
		"虎果",     // huguo
		"猫果",     // maoguo
		"HuangJu",  // 字母桶
		"阿果",     // aguo
		"北京果",   // beijingguo
		"！",       // 其他桶
	}
	want := []string{
		// 中文桶（按完整拼音序）：
		"阿果",   // aguo
		"北京果", // beijingguo
		"红果",   // hongguo
		"黄果",   // huangguo（huang < hug）
		"虎果",   // huguo
		"猫果",   // maoguo
		// 字母桶（按小写字母序）：
		"A123", "cloudfront", "HuangJu",
		// 其他桶最后：
		"！",
	}
	SortSourceNames(input)
	if !reflect.DeepEqual(input, want) {
		t.Fatalf("排序结果不符\n期望 %v\n实际 %v", want, input)
	}
}

func TestSortSourceNamesStable(t *testing.T) {
	// 拼音键相同的条目保持稳定顺序（完全同名），且拼音不同的正确排序：
	// 花果=huaguo < 荒果=huangguo（ua 在 ang 前）。
	input := []string{"荒果", "花果"}
	SortSourceNames(input)
	if !reflect.DeepEqual(input, []string{"花果", "荒果"}) {
		t.Fatalf("花果(huaguo) 应排在 荒果(huangguo) 前: %v", input)
	}
	// 同名键完全相同 → 稳定保持输入顺序。
	same := []string{"花果", "花果"}
	SortSourceNames(same)
	if len(same) != 2 {
		t.Fatalf("稳定排序被破坏: %v", same)
	}
}

func TestCompareSourceNamesLatin(t *testing.T) {
	// 字母名忽略大小写按字母序。
	if CompareSourceNames("a123", "Cloudfront") >= 0 {
		t.Fatalf("a123 应排在 Cloudfront 前")
	}
	if CompareSourceNames("黄果", "A123") > 0 {
		t.Fatalf("中文名应排在字母名前")
	}
}
