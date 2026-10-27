import 'package:pinyin/pinyin.dart';

/// 把标题里非「字母/数字」的字符清掉、转小写，用来做拼音首字母比对。
///
/// 例：`复仇者联盟` → `fczlm`，`A123` → `a123`，`流浪地球2` → `lldq2`。
String pinyinInitials(String title) => PinyinHelper.getShortPinyin(
  title,
).toLowerCase().replaceAll(RegExp(r'[^a-z0-9]'), '');

/// 判断 [query] 是否「只含字母/数字」（按纯拼音首字母走本地匹配的标志）。
/// 只要含一个中文字符就返回 false，让调用方改走远端子串联想。
bool isPinyinInitialsQuery(String query) {
  final trimmed = query.trim();
  if (trimmed.isEmpty) return false;
  for (final rune in trimmed.runes) {
    final c = String.fromCharCode(rune);
    if (!RegExp(r'[a-zA-Z0-9]').hasMatch(c)) return false;
  }
  return true;
}

/// 从 [titles] 里挑出「拼音首字母以 [query] 开头」的标题。
///
/// 用户输入 `fczlm` 会命中「复仇者联盟」，输入 `f` 会命中所有 f 开头的剧，
/// 输入越全结果越精确。最多返回 [limit] 条，去重后按拼音长度（越精确越靠前）。
List<String> matchPinyinInitials(
  String query,
  List<String> titles, {
  int limit = 10,
}) {
  final target = pinyinInitials(query);
  if (target.isEmpty) return const [];
  final scored = <(int, String)>[];
  final seen = <String>{};
  for (final raw in titles) {
    final title = raw.trim();
    if (title.isEmpty || seen.contains(title)) continue;
    final initials = pinyinInitials(title);
    if (initials.startsWith(target)) {
      seen.add(title);
      // 首字母越接近目标长度，说明越精确，排在前面。
      scored.add((initials.length, title));
    }
  }
  scored.sort((a, b) => a.$1.compareTo(b.$1));
  return scored.take(limit).map((e) => e.$2).toList();
}
