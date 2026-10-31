# CLAUDE.md

本文件是本仓库的常驻工作说明。每次改动核心链路后，回看并更新对应章节（尤其是“当前工作状态”和“红果取流”）。

## 项目概览

- 产品：全剧视界 / 短剧视界（同一代码库两个构建变体）。
- 技术栈：Flutter/Dart UI + Go 原生核心，通过 C FFI 调用（`native/bridge/main.go` ↔ `lib/core_bridge.dart`）。
- 平台：Android、Windows、iOS；另有本地 Web/服务器版 `native/server`。
- 主要目录：
  - `lib/`：Flutter 页面、播放器、下载、局域网、站源 UI。
  - `native/core/`：站源请求、解析、签名、取流、播放代理、下载。
  - `native/bridge/`：`c-shared` FFI 桥。
  - `native/server/`：本地服务器/Web 版。
  - `packages/`：打过补丁的 media_kit 相关包。
  - `scripts/`：各平台构建与打包脚本。
- 版本号在 `pubspec.yaml`；每个版本必须在 `CHANGELOG.md` 增加同名小节（标题如 `## 0.3.2+91`）。

## 构建与测试

本机工具链（2026-10-31）：

- Go 1.25.9：`E:\tools\go\bin`
- Flutter 3.47.9：`E:\tools\flutter\bin`
- JDK 17；Android SDK：`C:\Users\DELL\AppData\Local\Android\Sdk`（含 NDK、platform-tools）
- 这些目录不在默认 PATH，运行命令前先 `export PATH="/e/tools/go/bin:/e/tools/flutter/bin:$PATH"`。
- 本机 Flutter SDK 的 `flutter_tools` 有一段“无开发者模式时复制插件目录”的本地补丁。2026-10-31 已把它误用的 `Directory.copySync` 改为递归复制；重装 Flutter 后若 `flutter` 报同样错误，需要重新打这个补丁，否则工具快照无法生成。
- 本机访问 pub.dev / services.gradle.org 不稳定。依赖已缓存时可跳过 `pub get`，直接：
  `/c/Python311/python.exe scripts/build_android.py` 会先跑 `flutter pub get`；网络失败时改用手动命令
  `flutter build apk --release --split-per-abi --no-pub --dart-define=ALL_SOURCES=true --target-platform android-arm64`。
  Gradle 9.3.1 发行包可从 `https://repo.huaweicloud.com/gradle/gradle-9.3.1-all.zip` 补到
  `~/.gradle/wrapper/dists/gradle-9.3.1-all/<hash>/`（同时创建 `.zip.ok`）。
- `android/gradle.properties` 已设置 `kotlin.incremental=false`：本机 pub 缓存在 C:、工程在 E:，
  Kotlin 增量缓存无法跨盘符相对化源文件（`different roots`），不关闭会构建失败。md 依赖缓存也常需
  从 `ghproxy.net` 补下载 media_kit 的 libmpv jar 到 `build/media_kit_libs_android_video/v1.1.7/`。

```bash
# Android（全源版 = 全剧视界；默认版是短剧视界）
# 默认 python 是 3.6.5，脚本需要 3.7+；本机用 C:\Python311\python.exe
/c/Python311/python.exe scripts/build_android.py --abi arm64-v8a --all-sources

# Go 测试（在 native/ 下）
go test ./core ./bridge ./server
go vet ./core
gofmt -l core

# Flutter
flutter analyze
flutter test
```

- Android 签名：`android/key.properties` + `android/app/quanjushijie-release.jks`（不入库）；`applicationId = com.duanju.duanju_app`。
- 覆盖升级必须同证书；否则先导出配置备份，再卸载/安装/恢复。
- 不要手工提交 `dist/` 或生成的 `jniLibs`。

## 代码约定

- 红果相关文件多为 `// SPDX-License-Identifier: GPL-3.0-only`，保留该头。
- 用户可见错误由各 provider 的 `publicError`/专用文案生成；诊断日志会脱敏 URL 查询参数。**不得**在日志或 UI 泄露 `device_id`、`install_id`、`cdid`、签名头、完整媒体 URL/query。
- 原生错误尽量带 `errors.Is` 可判定的 sentinel/typed error；重试必须有界。
- 改动签名算法、请求体、查询参数顺序、注册包加密格式属于高风险，非必要不要动。
- 站源/播放改动要同时考虑后台下载、预加载、局域网播放，不要只修播放页。

## 红果取流（重点）

链路（`native/core/`）：

1. 匿名播放设备注册：`provider_hongguo_playback_device.go`（`log5-applog-lf.fqnovel.com`，gzip + SHA-512 + AES-128-CBC；身份仅进程内缓存）。
2. 批量取流：`provider_hongguo_signed_media.go`（`/novel/player/multi_video_model/v1/`，六签名）。
3. 加密/解密：`provider_hongguo_spade.go`、`provider_hongguo_fallback_media.go`（`key_seed`、`spade_a`、CENC key）。
4. 降级：匿名注册 → 兼容注册 → 旧单集接口 → 预加载 → 官网网页。
5. 播放：本地 `127.0.0.1` 代理（`app_stream.go`），再由 libmpv 拉流。

关键约束：

- 没有账号/Cookie/session，不要引入登录或持久化设备号。
- 不要恢复已下线的 `djapi.999888456.xyz`。
- 失败分类在 `provider_hongguo_playback_errors.go`：
  - 可重签：空正文、401、非 0 code、格式异常、无 `video_model`。
  - 换新设备号：上述失败在重签后仍失败时，最多一次、30 秒冷却。
  - 不重签/不换号：403/429（limiter backoff）、5xx、TLS/EOF、缺分集。
- ALPN 重试在 `network_hongguo_alpn.go`，由 `network_cdn.go` 的 Hongguo 分支调用。
- 播放代理的 `tpN` CDN 边缘轮换只在实测媒体 host 确认为 `tpN.*novelvod.com` 形式后启用，原 host 永远优先。

## 当前工作状态（2026-10-31）

用户：Android 旧版全剧视界，红果“取流失败打不开”。已批准计划：
`C:\Users\DELL\.claude\plans\c-users-dell-documents-xwechat-files-wx-glistening-kettle.md`。

已完成（Phase 1）：

- 注册失败改为 15s 起、上限 2min 的冷却重试；`forgetHongguoPlaybackDevice` 可安全换身份。
- 六签名触发面扩大 + 失败后最多换一次设备号（有界：3 次播放请求、2 次注册）。
- 错误分类/用户文案集中到 `provider_hongguo_playback_errors.go`。
- ALPN 重试接入 `cdnTransport`；移除已下线的 `djapi` host。
- 新增本文件。

已完成（Phase 2）：

- `tpN` CDN 边缘轮换（`network_hongguo_media_edge.go` + `app_stream.go`）：原 host 优先，失败边 30s 退避，成功边记忆 10min；非 `tpN` 地址行为不变。
- Android `vpnActive`/`bindPhysicalNetwork` 已在 `MainActivity.kt` 实现；`PlaybackLoader` 对可重试出口错误借用物理网络并重试一次；Go 新增 `resetNetwork`（关闭空闲连接 + 清红果 backoff）。
- 日记过滤加入 `hongguo.playback.*` 与 `network.alpn_retry`；`stream.cdn_edge` 记录边缘轮换。
- Go/Dart 测试已补（注册冷却/单飞/换身份、重试分类、ALPN、边缘、backoff、NetworkExit）。

已完成（Phase 3）：

- `go test ./core` 新增用例全过（仅剩两个既有失败）；`go vet ./core` 干净。
- `flutter analyze` 仅剩 4 条既有 info；新增 Dart 测试 9/9 通过；完整 `flutter test` 仅剩 `rankings_screen_test.dart` 既有失败。
- 版本 `0.3.3+92`、README/CHANGELOG 已更新。
- Android arm64 全源版 APK 构建成功：`dist/android/quanjushijie-0.3.3+92-arm64-v8a.apk`（签名 `CN=quanjushijie`）。
- 真机实测（Xiaomi 2407FRK8EC，Android 16）：红果列表正常，第 1、48 集可播放；日记显示 `register_started` → `register_ready` → `attempt`（unsigned，171ms，1080P）→ `registered_resolved`，媒体 host `dj-reading-video-x.qznovelvod.com`。实测 host 非 `tpN`，边缘轮换未触发。

当前状态（2026-10-31）：

- 已提交 `f48780c` 并 push 到 `origin/main`（`https://github.com/qyf041107/guoapp_selfuse-.git`）。
- 已创建 GitHub Release `app-v0.3.3-92`，附 Android arm64 APK：
  `https://github.com/qyf041107/guoapp_selfuse-/releases/tag/app-v0.3.3-92`
- 真机实测通过：红果列表、第 1/48 集播放、注册/取流日记均正常。

注：`CLAUDE.md` 在 `.gitignore` 中（第 43 行），默认不会随代码提交；如需入库要先调整 `.gitignore`。
