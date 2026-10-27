package core

import (
	"sort"
	"strings"

	"github.com/mozillazg/go-pinyin"
)

// 站源名称排序规则：先中文名（按汉字拼音序），再字母/拉丁名（按字母序），其余排最后。
// 中文名的拼音键取每个汉字首音（无声调、小写），多音字取默认音。

// sourceNameClass 返回名称所属排序桶：0=中文、1=字母、2=其他。
func sourceNameClass(name string) int {
	for _, rune := range name {
		if rune >= 0x4e00 && rune <= 0x9fff {
			return 0
		}
		if (rune >= 'a' && rune <= 'z') || (rune >= 'A' && rune <= 'Z') {
			return 1
		}
	}
	// 全空名归最后。
	return 2
}

// pinyinKey 生成排序键：
//   - 中文名：转小写拼音串（无音调），例「红果」→「hongguo」；
//   - 字母名：原样转小写；
//   - 其他：原样（作为桶内兜底）。
func pinyinKey(name string) string {
	trimmed := strings.TrimSpace(name)
	if sourceNameClass(trimmed) != 0 {
		return strings.ToLower(trimmed)
	}
	arguments := pinyin.NewArgs()
	arguments.Heteronym = false
	arguments.Style = pinyin.Normal
	syllables := pinyin.LazyPinyin(trimmed, arguments)
	joined := make([]string, 0, len(syllables))
	for _, syllable := range syllables {
		joined = append(joined, strings.ToLower(syllable))
	}
	return strings.Join(joined, "")
}

// CompareSourceNames 按「先中文（拼音序）后字母序再其他」比较两个站源名称。
// 同桶内按拼音/字母键比较；键相等返回 0（由调用方稳定排序保持原顺序）。
func CompareSourceNames(a, b string) int {
	classA, classB := sourceNameClass(a), sourceNameClass(b)
	if classA != classB {
		return classA - classB
	}
	keyA, keyB := pinyinKey(a), pinyinKey(b)
	switch {
	case keyA == keyB:
		return 0
	case keyA < keyB:
		return -1
	default:
		return 1
	}
}

// SortSourceNames 按 CompareSourceNames 对名称列表做稳定排序（同键保持输入顺序）。
func SortSourceNames(names []string) {
	sort.SliceStable(names, func(i, j int) bool {
		return CompareSourceNames(names[i], names[j]) < 0
	})
}
