# duanju patch: media_kit_video 2.0.1 (Windows)

本目录是 `media_kit_video` **2.0.1** 的本地副本（通过 `pubspec.yaml` 的
`dependency_overrides` 覆盖）。除 `windows/` 下的 C++ 补丁外与上游完全一致，
Dart / Android / iOS / Linux / macOS 代码未做任何修改。

## 为什么打补丁

Windows 版「播放几个视频后闪退」，崩溃现场（WER dump）：

- 异常码 `0xc0000409`（`__fastfail`）
- 栈底 `media_kit_video_plugin.dll` → `msvcp140!std::_Xout_of_range`
- `std::out_of_range` 无人捕获 → `std::terminate` → 进程被系统强制结束

根因在 `windows/video_output.cc` + `windows/thread_pool.h`：

1. `Render()` / `CheckAndResize()` 被投递到 thread pool 执行，里面直接调用
   `pixel_buffer_textures_.at(texture_id_)`。`std::unordered_map::at()` 在 key
   不存在时抛 `std::out_of_range`。
2. `Resize()`（每次视频分辨率变化都会触发）会先把 `texture_id_` 清 0、注销旧
   纹理并 `erase` 旧表项，然后才注册新纹理。这个过程中 `texture_id_` 与两张
   纹理表不是原子地一起更新，**渲染线程可能读到「id 已换新 / 表项还没插进去」
   或「id 已清 0」的中间状态** —— 于是 `.at()` 抛异常。
3. `thread_pool.h` 里 `task()` 是裸调用，异常逃出工作线程 → `std::terminate()`
   → `0xc0000409`。这就是「闪退且什么都不留下」的原因（不是 Go panic，Go 的
   `recover()` 拦不到 C++ 异常）。

「播几个视频就崩」正好对应「每换一集/切清晰度都会 resize 一次」的调用频率。

## 改了哪些地方

全部改动均以 `[duanju patch]` 注释标记。

- `windows/thread_pool.h`
  - 工作线程里 `task()` 包 `try/catch(...)`：任何任务抛异常都不再 `terminate`
    进程，最坏情况只是丢一帧。
- `windows/video_output.cc`
  - `Render()`：先把 `texture_id_` 快照到局部变量；S/W 分支在 `textures_mutex_`
    下用 `find()` 查表，查不到就丢帧返回；整体包 `try/catch`。
  - `CheckAndResize()`：`find()` 查表 + 查不到直接返回；整体包 `try/catch`。
  - `Resize()`：`texture_id_` 的置 0 与「新纹理入表后发布新 id」全部在
    `textures_mutex_` 内完成，**先入表、后发布 id**，消除中间状态的可见窗口；
    `texture_update_callback_` 移到锁外调用。
  - 两个纹理回调（H/W 的 `GpuSurfaceTexture`、S/W 的 `PixelBufferTexture`）里的
    `.at()` 改成 `find()`，查不到返回 `nullptr`（上游代码本身就把 `nullptr`
    当作「本帧无纹理」处理）。
  - `~VideoOutput()`：`promise.set_value()` 保证一定执行（原来若 teardown 抛异常
    或 `texture_id_ == 0`，`wait()` 会永久阻塞，把 `VideoOutputManager` 的互斥锁
    一直占住，之后所有 Create/Dispose 全部卡死）；`texture_id_ = 0` 加锁。
- `windows/video_output.h`
  - `width()` / `height()` 的 `.at()` 改成 `find()` + 空值回退。

## 维护方式

上游后续版本若修复了该问题，把 `pubspec.yaml` 里的 `media_kit_video` 覆盖去掉、
恢复成版本号即可。升级前请先确认上游 `windows/video_output.cc` 里不再有裸
`.at(texture_id_)` 调用。
