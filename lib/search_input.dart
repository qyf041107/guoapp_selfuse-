import 'dart:async';
import 'dart:math';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'pinyin_search.dart';

class SearchInput extends StatefulWidget {
  const SearchInput({
    super.key,
    required this.controller,
    required this.hint,
    required this.onSearch,
    this.onChanged,
    this.suggestions,
    this.localTitles,
    this.onCancel,
    this.autofocus = false,
    this.crossSource = false,
    this.onCrossSourceChanged,
    this.crossSourceCount = 0,
  });
  final TextEditingController controller;
  final String hint;
  final ValueChanged<String> onSearch;
  final ValueChanged<String>? onChanged;
  final Future<List<String>> Function(String)? suggestions;
  // 本地片库里的剧名（收藏 / 最近观看 / 下载）。输入纯字母时按拼音首字母
  // 在本地即时匹配（如 fczlm → 复仇者联盟），离线、隐私不出端；没命中再走远端。
  final List<String>? localTitles;
  final VoidCallback? onCancel;
  final bool autofocus;
  // crossSource 勾选后并发搜索所有可见站源，否则只搜索当前站源。
  final bool crossSource;
  final ValueChanged<bool>? onCrossSourceChanged;
  final int crossSourceCount;
  @override
  State<SearchInput> createState() => _SearchInputState();
}

class _SearchInputState extends State<SearchInput> {
  final _focus = FocusNode(debugLabel: 'search-input');
  Timer? _timer;
  Completer<Iterable<String>>? _pending;
  int _generation = 0;
  final _cache = <String, List<String>>{};
  final _nav = ValueNotifier<int>(0);
  List<String> _lastOptions = const [];

  @override
  void initState() {
    super.initState();
    _focus.addListener(_onFocusChanged);
    _focus.onKeyEvent = _handleKey;
  }

  bool _sameOptions(List<String> a, List<String> b) {
    if (a.length != b.length) return false;
    for (var i = 0; i < a.length; i++) {
      if (a[i] != b[i]) return false;
    }
    return true;
  }

  void _syncOptions(List<String> result) {
    if (!_sameOptions(result, _lastOptions)) _nav.value = 0;
    _lastOptions = result;
  }

  void _onFocusChanged() {
    if (!_focus.hasFocus) _nav.value = 0;
  }

  bool get _dropdownOpen =>
      _focus.hasFocus && widget.controller.text.trim().isNotEmpty;

  // 输入法候选（拼音组字）进行中时不劫持方向键，交由软键盘选词。
  bool get _composing => !widget.controller.value.composing.isCollapsed;

  KeyEventResult _handleKey(FocusNode node, KeyEvent event) {
    if (event is! KeyDownEvent && event is! KeyRepeatEvent) {
      return KeyEventResult.ignored;
    }
    if (!_focus.hasFocus) return KeyEventResult.ignored;
    final key = event.logicalKey;
    if (_composing) return KeyEventResult.ignored;
    final count = _lastOptions.length;
    if (key == LogicalKeyboardKey.arrowUp) {
      if (_dropdownOpen && count > 0 && _nav.value > 0) {
        _nav.value--;
        return KeyEventResult.handled;
      }
      return _focus.focusInDirection(TraversalDirection.up)
          ? KeyEventResult.handled
          : KeyEventResult.ignored;
    }
    if (key == LogicalKeyboardKey.arrowDown) {
      if (_dropdownOpen && count > 0 && _nav.value < count - 1) {
        _nav.value++;
        return KeyEventResult.handled;
      }
      return _focus.focusInDirection(TraversalDirection.down)
          ? KeyEventResult.handled
          : KeyEventResult.ignored;
    }
    if (_dropdownOpen &&
        count > 0 &&
        (key == LogicalKeyboardKey.enter ||
            key == LogicalKeyboardKey.numpadEnter ||
            key == LogicalKeyboardKey.select ||
            key == LogicalKeyboardKey.gameButtonA)) {
      _submit(_lastOptions[_nav.value.clamp(0, count - 1)]);
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  void _abort() {
    _timer?.cancel();
    widget.onCancel?.call();
    _generation++;
    if (_pending != null && !_pending!.isCompleted) {
      _pending!.complete(const []);
    }
  }

  void _cancel() {
    _abort();
    _syncOptions(const []);
    _nav.value = 0;
  }

  @override
  void didUpdateWidget(SearchInput oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.suggestions == null) _cancel();
  }

  @override
  void dispose() {
    _abort();
    _nav.dispose();
    _focus.dispose();
    super.dispose();
  }

  // 纯字母输入（拼音首字母）：优先在本地片库即时匹配，离线、不出端。
  // 命中直接返回；没命中才退回远端（远端按中文子串匹配，纯字母多半无果，仅兜底）。
  List<String> _localOptions(String query) {
    final titles = widget.localTitles;
    if (titles == null || titles.isEmpty) return const [];
    return matchPinyinInitials(query, titles, limit: 10);
  }

  Future<Iterable<String>> _options(TextEditingValue value) async {
    _abort();
    final query = value.text.trim();
    if (query.isEmpty || query.length > 100 || !value.composing.isCollapsed) {
      _syncOptions(const []);
      return const [];
    }
    final cached = _cache[query];
    if (cached != null) {
      _syncOptions(cached);
      return cached;
    }
    // 首字母提示：输入 fczlm 先在本地的收藏 / 看过的片库里找「复仇者联盟」。
    if (isPinyinInitialsQuery(query)) {
      final local = _localOptions(query);
      if (local.isNotEmpty) {
        _remember(query, local);
        _syncOptions(local);
        return local;
      }
      if (widget.suggestions == null) {
        _syncOptions(const []);
        return const [];
      }
    }
    if (widget.suggestions == null) {
      _syncOptions(const []);
      return const [];
    }
    final generation = _generation;
    final pending = Completer<Iterable<String>>();
    _pending = pending;
    _timer = Timer(const Duration(milliseconds: 300), () async {
      var result = const <String>[];
      try {
        final fetched = (await widget.suggestions!(
          query,
        )).where((s) => s.trim().isNotEmpty).toSet().take(10).toList();
        if (generation == _generation && mounted) {
          _remember(query, fetched);
          result = fetched;
        }
      } catch (_) {}
      if (!pending.isCompleted) pending.complete(result);
      if (generation == _generation && mounted) _syncOptions(result);
    });
    return pending.future;
  }

  void _remember(String query, List<String> items) {
    if (_cache.length >= 32) _cache.remove(_cache.keys.first);
    _cache[query] = items;
  }

  void _submit(String value) {
    _cancel();
    widget.controller.value = TextEditingValue(
      text: value,
      selection: TextSelection.collapsed(offset: value.length),
    );
    _focus.unfocus();
    widget.onSearch(value.trim());
  }

  Widget _highlight(String value) {
    final query = widget.controller.text.trim().toLowerCase();
    final spans = <TextSpan>[];
    var start = 0;
    while (query.isNotEmpty) {
      final match = value.toLowerCase().indexOf(query, start);
      if (match < 0) break;
      if (match > start) {
        spans.add(TextSpan(text: value.substring(start, match)));
      }
      spans.add(
        TextSpan(
          text: value.substring(match, match + query.length),
          style: TextStyle(
            color: Theme.of(context).colorScheme.primary,
            fontWeight: FontWeight.bold,
          ),
        ),
      );
      start = match + query.length;
    }
    if (start < value.length) spans.add(TextSpan(text: value.substring(start)));
    return Text.rich(
      TextSpan(children: spans),
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) => RawAutocomplete<String>(
      textEditingController: widget.controller,
      focusNode: _focus,
      optionsBuilder: _options,
      onSelected: (value) => _submit(value),
      fieldViewBuilder: (context, controller, focus, submitted) => TextField(
        controller: controller,
        focusNode: focus,
        autofocus: widget.autofocus,
        textInputAction: TextInputAction.search,
        onChanged: (value) {
          widget.onChanged?.call(value);
          setState(() {});
        },
        onSubmitted: (value) {
          if (_dropdownOpen && _lastOptions.isNotEmpty) {
            _submit(_lastOptions[_nav.value.clamp(0, _lastOptions.length - 1)]);
          } else {
            _submit(value);
          }
        },
        decoration: InputDecoration(
          hintText: widget.hint,
          prefixIcon: const Icon(Icons.search_rounded),
          suffixIcon: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (controller.text.isNotEmpty)
                IconButton(
                  tooltip: '清空搜索',
                  icon: const Icon(Icons.close_rounded),
                  onPressed: () {
                    _cancel();
                    controller.clear();
                    widget.onChanged?.call('');
                    setState(() {});
                  },
                ),
              if (widget.onCrossSourceChanged != null)
                Tooltip(
                  message: widget.crossSourceCount > 0
                      ? '搜索可见源：并发搜索 ${widget.crossSourceCount} 个站源并去重'
                      : '搜索可见源',
                  child: InkWell(
                    borderRadius: BorderRadius.circular(8),
                    onTap: () =>
                        widget.onCrossSourceChanged!(!widget.crossSource),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 6,
                        vertical: 8,
                      ),
                      child: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          SizedBox(
                            width: 18,
                            height: 18,
                            child: Checkbox(
                              value: widget.crossSource,
                              visualDensity: VisualDensity.compact,
                              materialTapTargetSize:
                                  MaterialTapTargetSize.shrinkWrap,
                              onChanged: (value) =>
                                  widget.onCrossSourceChanged!(value ?? false),
                            ),
                          ),
                          const SizedBox(width: 2),
                          const Text('全部源', style: TextStyle(fontSize: 12)),
                        ],
                      ),
                    ),
                  ),
                ),
              IconButton(
                tooltip: '搜索',
                icon: const Icon(Icons.arrow_forward_rounded),
                onPressed: () => _submit(controller.text),
              ),
            ],
          ),
        ),
      ),
      optionsViewBuilder: (context, selected, options) {
        final values = options.toList();
        final available =
            MediaQuery.sizeOf(context).height -
            MediaQuery.viewInsetsOf(context).bottom;
        return Align(
          alignment: Alignment.topLeft,
          child: Material(
            elevation: 8,
            borderRadius: BorderRadius.circular(14),
            clipBehavior: Clip.antiAlias,
            child: SizedBox(
              width: constraints.maxWidth,
              height: min(
                min(300.0, max(96.0, available * .45)),
                values.length * 48.0,
              ),
              child: ValueListenableBuilder<int>(
                valueListenable: _nav,
                builder: (context, index, _) => ListView.builder(
                  padding: EdgeInsets.zero,
                  itemExtent: 48,
                  itemCount: values.length,
                  itemBuilder: (context, item) => ListTile(
                    key: ValueKey('search-suggestion-$item'),
                    selected: item == index,
                    leading: const Icon(Icons.search, size: 18),
                    title: _highlight(values[item]),
                    onTap: () => _submit(values[item]),
                  ),
                ),
              ),
            ),
          ),
        );
      },
    ),
  );
}
