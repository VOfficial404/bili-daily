# NSGo feasibility spike: Windows 11 x64 host, Go 1.27.3 only

Windows x64, web and Android arm64 all build on this host with Go alone, and the Android APK comes out signed. iOS cannot be built here (needs macOS + Xcode), and NSGo has no background-execution support on either mobile OS. Nothing was run on a device, emulator or browser, so runtime behaviour is unverified.

- **Version:** `github.com/nsgo-dev/nsgo v0.0.0-20261112150101-70cef6a4ed17` (pseudo-version, no release; commit `70cef6a4ed17f7c8428faabc9bb419ff4223745b`, 2026-11-12). License is Apache-2.0.
- **Go requirement:** NSGo's `go.mod` says `go 1.26.0` and `cmd/nsgo/doctor.go` has `minGo = "go1.26"`. `go mod init` wrote `go 1.27.3` into the app's `go.mod` (host toolchain).
- **Host shell quirk:** `PATH` is `;`-separated Windows style, so `PATH="$PWD/.tools/go/bin:$PATH"` does not work; the Go directory has to be prepended as a Windows path with `;`.

## (a) Build matrix

All commands were run in `.tools/spike-nsgo/hello/` with `NO_COLOR=1`. Builds took 5–10 s each.

| Target | Command | Result | Artifact (bytes) | Extra prerequisites |
|---|---|---|---|---|
| `windows_x64` | `go tool nsgo build -target windows_x64` | OK | `dist/hello-0.1.0-windows_x64/hello.exe` (8,312,832; PE32+ GUI) and `hello.nsi` (5,093) | None for the exe. The `-setup.exe` installer needs `makensis`; without it only the `.nsi` script is left. |
| `web` | `go tool nsgo build -target web` | OK | `dist/hello-0.1.0-web/` (`app.78bf7380d4.wasm` 11,289,678, plus `index.html`, `nsgo.*.js`, `wasm_exec.*.js`, `nsgo.json`, `manifest.webmanifest`); `dist/releases/hello-0.1.0-web.zip` (3,235,362) | None |
| `android_arm64` | `go tool nsgo build -target android_arm64` | OK, signed | `dist/hello-0.1.0-android_arm64/hello.apk`, copied to `dist/releases/hello-0.1.0-android-arm64.apk` (3,340,471) with `.sha256` | None: no JDK, SDK or NDK |
| `android_arm64`, release key | `NSGO_KEYSTORE_PASSWORD=… NSGO_KEY_PASSWORD=… go tool nsgo build -target android_arm64 -keystore ../keystore/release.p12 -output dist-signed` | OK | `dist-signed/.../hello.apk` (3,340,415) | A PKCS#12 or JKS keystore with an RSA key (I made a throwaway one with Git's `openssl`) |
| `android_arm64`, store | same plus `-channel store -output dist-store` | OK | `dist-store/releases/hello-0.1.0-android-arm64.aab` (3,339,599) | Keystore |
| `linux_x64` | `go tool nsgo build -target linux_x64 -output dist-extra` | OK | `dist-extra/hello-0.1.0-linux_x64/hello` (7.9 MB), plus `.deb`, `.AppImage`, `.pkg.tar.gz` in `dist-extra/releases/` | None |
| `android_x64` | `… -target android_x64 …` | Fails | none | Android NDK |
| `ios`, `ios_simulator` | `… -target ios …` | Fails | none | macOS + Xcode |
| `macos_arm64` | `… -target macos_arm64 …` | Fails | none | `codesign` (macOS), although `doctor` marks it buildable |

Output excerpts:

```
  ✓ built windows_x64 (6.5s, 7.9 MB)
    install NSIS (brew install makensis) and compile dist\hello-0.1.0-windows_x64\hello.nsi for the installer
  warn no install scripts: set release.github in nsgo.yaml to the repository whose releases they install from
  ✓ built web (5.2s, 10.8 MB)
    → dist\releases\hello-0.1.0-web.zip
    signed with the debug key: set sign.keystore for store releases
  ✓ built android_arm64 (6.5s, 3.2 MB)
    → dist\releases\hello-0.1.0-android-arm64.apk
  error ios: the iOS SDK needs Xcode: xcrun --sdk iphoneos --show-sdk-path: exec: "xcrun": executable file not found in %PATH%
  error android_x64: the x86_64 emulator target needs an Android NDK: install one with sdkmanager "ndk;27.2.12479018" and set ANDROID_NDK_HOME; arm64 devices and emulators need none
  error macos_arm64: codesign --force --sign - dist-extra\hello-0.1.0-macos_arm64\hello.app: exec: "codesign": executable file not found in %PATH%
```

`go tool nsgo doctor` on this host reported: Go 1.27.3 found; no adb, no NDK, no makensis; Chrome found. It marks all desktop targets, `android_arm64` and `web` as buildable, and every Apple target as "needs a Mac with Xcode".

`go tool nsgo test` passed headless (`✓ TestPress`, 1 passed) and `go tool nsgo check` was clean.

### Is the Android APK installable and signed?

Yes, as far as can be checked without a device. It was not installed anywhere; there is no adb, apksigner or Java here.

- **Contents:** `AndroidManifest.xml`, `classes.dex` (351,880, the prebuilt Java runtime embedded in the tool), `lib/arm64-v8a/libnsgo.so` (8,257,830), `resources.arsc`. There is no `META-INF`.
- **Signature:** APK Signature Scheme v2 only (RSA PKCS#1 v1.5 SHA-256), written in pure Go by `cmd/nsgo/apk/sign.go`. v2-only is valid because `minSdkVersion` is 28.
- **Independent check:** I wrote `.tools/spike-nsgo/verify-apk/main.go`, which recomputes the chunked digest and verifies the RSA signature. Result for the debug-key, release-key and probe APKs:
  ```
  cert subject="CN=Android Debug,O=Android,C=US" notAfter=2126-11-13 rsaBits=2048
  content digest matches: true
  signature verifies: true
  OK: valid APK Signature Scheme v2 signature
  ```
- **Debug key:** generated on first build at `<project>/.nsgo/android/debug.pem`. Android rejects updates signed by a different key, so persist this file or use `sign.keystore` (CI included).
- **Manifest template** (`internal/platform/android/runtime/AndroidManifest.xml`): `minSdkVersion=28`, `targetSdkVersion=34`, `INTERNET` and `ACCESS_NETWORK_STATE`, `extractNativeLibs=true`, one activity `dev.oneo.nsgo.NsgoActivity`, one `FileProvider`.

### iOS (not buildable here)

- **Needs:** macOS + Xcode. `iosEnv` in `cmd/nsgo/ios.go` shells out to `xcrun` for the SDK and clang, and builds with `GOOS=ios GOARCH=arm64 CGO_ENABLED=1`. Minimum iOS is 15.0.
- **Command:** `go tool nsgo build -target ios`. Per `build.go` it writes `dist/<name>-<version>-ios/<name>.app` and `<name>.ipa`, copied to `dist/releases/…-ios.ipa`.
- **Unsigned IPA:** yes. Without `sign.profile`, `signForStore` (`cmd/nsgo/sign.go:171`) prints `unsigned: set sign.profile to install it on devices or upload it` and still packages the IPA. Signed builds use `sign.profile` (a `.mobileprovision`) with `sign.identity` or the keychain identity.
- **CI:** `macos-latest` with `actions/setup-go` should do it. The repo's own workflow uses that runner, but only for `check`/`test`, and its push triggers are currently disabled.

### Android on Linux CI

Should work on `ubuntu-latest` with Go alone, but I only proved it on Windows.

- The arm64 path is `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build`, with the dex embedded and the manifest, zip and signature written in Go.
- `cmd/nsgo/android.go` has no host-OS checks, and `doctor`'s `build()` returns yes for non-Apple targets on any host.
- The docs only say "every other target builds from macOS with Go alone".

## (b) Generated `nsgo.yaml` and skeleton

`go tool nsgo init` printed `✓ create app.go`, `✓ create app_test.go`, `✓ create nsgo.yaml`, `next: go tool nsgo dev .`

| File | Bytes | Content |
|---|---|---|
| `app.go` | 1,065 | Counter: `Root(cx *core.Scope) core.Node` and `func main() { nsgo.Run(Root) }` |
| `app_test.go` | 331 | `nsgotest.Main(m)`; `nsgotest.Start(t, Root)`, `app.Press("role:button Press me")`, `app.Find("text:Pressed 1 times")` |
| `nsgo.yaml` | 1,174 | below |
| `go.mod`, `go.sum` | 670 / 2,193 | from `go get`; ends with `tool github.com/nsgo-dev/nsgo/cmd/nsgo` |

Created by builds: `.nsgo/android/debug.pem` and `dist/`.

```yaml
name: hello
id: dev.nsgo.hello
version: 0.1.0

# (comment block about icons: icon.apple / icon.png / icon.tvos / icon.watchos, defaults under assets/)

dev:
  # native: this machine's backend; web: the browser; ios: a connected iPhone; ios_simulator; android: a connected device or emulator.
  target: native
  # true, the default, rebuilds and restarts on save; false runs once and exits with the app.
  hot_reload: true

build:
  output: dist
  # true opens the output directory after building.
  open: false
  targets: [macos_arm64, windows_x64, ios_simulator, android_arm64, web]
```

Other keys confirmed in `cmd/nsgo/project.go` and `docs/public/en/building.md`:

- **App:** `build_number`, `appearance: light|dark`, `icon.{apple,png,tvos,watchos}`, `i18n: {dir, source}`.
- **Build:** `build.{installers,jobs}`, `release.{github,tag_prefix}`, `updates.{key,channel,feed,automatic}`.
- **Android:** `android.permissions` (a name without a dot gets the `android.permission.` prefix) and `android.target_sdk`. Nothing else is accepted under `android:`.
- **Apple:** `apple.info` (string values only) and `apple.app_group`.
- **Signing:** `sign.{identity,profile,keystore,key_alias,notarize,…}`. Keystore passwords come only from `NSGO_KEYSTORE_PASSWORD` and `NSGO_KEY_PASSWORD`.
- **Other:** `mobile.exclude_resources`, `windows.{installer_icon,uninstaller_icon,store}`, `macos.*`, `linux.*`.

Files under `resources/` ship with every package (APK `assets/`, beside the exe, beside the web page) and load as `asset:` paths.

## (c) API cheat-sheet

Imports: `github.com/nsgo-dev/nsgo` plus `/core`, `/base`, `/ui`, `/mobile`, `/desktop`, `/icons/lucide`, `/nsgotest`, `/updater`.

Everything marked ✔ is type-checked in `.tools/spike-nsgo/probe/app.go`, which vets clean and builds for `windows_x64`, `android_arm64` and `web` (exe 13.2 MB, APK 4.85 MB, wasm 20.2 MB / 5.4 MB zipped).

**Basics** ✔
```go
func Root(cx *core.Scope) core.Node { ... }
func main() { nsgo.Run(Root) }
count := core.NewSignal(cx, 0)              // .Get() .Set(v) .Update(fn); NewSignalFunc(cx, v, equal) for non-comparable types
cx.Effect(func() { ... })                   // re-runs when the signals it read change
core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(24), PaddingEnv: core.EnvAll(core.EnvSafeArea), Gap: 16}, Children: []core.Node{...}}
core.Text{Content: "x"}  core.Text{ContentFunc: func() string {...}}
core.Show{When: func() bool {...}, Then: func(*core.Scope) core.Node {...}, Else: ...}
ui.Text("Title", 20, ui.Medium, colors.Text)   ui.Textf(sig.Get, 12, ui.Regular, colors.Danger)   // colors := ui.ColorsOf(cx)
```
Goroutines must reach signals through `cx.Post(func(){ sig.Set(v) })`. `net/http` works on every target.

**Tab bar** ✔
```go
mobile.TabBar{
    Items:    []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
    Selected: func() int { return tab.Get() },
    OnChange: func(i int) { tab.Set(i) },
}
```
- It is the system bar on iOS and Android and takes no room elsewhere; `mobile.IsAvailable(cx)` says whether it shows. Use `ui.Tabs[T]` or `ui.TabShell` on desktop and web.
- `Icon` is an SF Symbol name on iOS. On Android it is "a drawable of the app" or an `android:`-prefixed system drawable such as `"android:ic_menu_search"`.
- Other fields: `ItemsFunc`, `Style`, `TestID`.

**Navigation stack** ✔
```go
nav := base.NewNav(cx, "inbox")   // Push(r) Pop(n) PopToRoot() Replace(r) SetRoutes(...) Depth()
return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
    if route == "inbox" {
        return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{...}}, ui.NavHeader{})
    }
    return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
        OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
        Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
    })
})
```
`base.NavPage` is `core.Screen` (`Title`, `TitleFunc`, `HeaderHidden`, `HeaderLargeTitle`, `NoPopOnBack`, `Children`, …). `cx.OnBackPress(func() bool)` intercepts back.

**Forms** ✔
```go
ui.Input{Value: name.Get, OnChange: name.Set, Placeholder: "Your name", Label: "Name",
    ReturnKeyType: core.ReturnKeyNext, OnSubmit: func(string) { next.Focus() }}          // Ref: &core.Ref{}
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set, AutoComplete: core.AutoCompletePassword}   // password field
ui.Switch{Checked: on.Get, OnChange: on.Set, Label: "Notifications"}                      // Size: ui.SizeSmall|SizeDefault|SizeLarge
ui.Select[string]{
    Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}},
    Value: size.Get, OnChange: size.Set, Label: "Size",
    Presentation: base.PresentSheet,   // also Searchable, Placeholder, OnClear
}
ui.Button{Text: "Save", OnPress: save}   // Variant: ui.ButtonPrimary|ButtonSecondary|ButtonOutline|ButtonGhost; Icon: lucide.Check; Disabled: func() bool
ui.FormSection{Title: "Account", Footer: "...", Children: []core.Node{
    ui.FormRow{Title: "Dark mode", Detail: "", Control: ui.Switch{...}},
    ui.FormRow{Title: "Log", Icon: lucide.List, OnPress: open, Variant: ui.FormRowDanger},
}}
```
- More input fields: `Multiline`, `Readonly`, `Disabled`, `MaxLength`, `KeyboardType: core.KeyboardEmail|KeyboardNumber|KeyboardURL`, `DefaultValue`, `OnFocus`/`OnBlur`.
- Keyboard: `PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard}` and `cx.DismissKeyboard()`.
- Also available: `ui.Checkbox`, `ui.Radio`/`ui.RadioGroup`, `ui.NumberInput`, `ui.OtpInput`, `ui.Settings`.

**Lists and scroll** ✔
```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
    core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows}}}
core.List{
    Count: func() int { return len(items) }, Key: func(i int) any { return items[i].ID }, EstimatedSize: 56,
    Row: func(cx *core.Scope, r core.ListRow) core.Node { return core.Text{Content: items[r.Index()].Title} },
}
```
- Scroll: `OnRefresh` and `Refreshing` give pull-to-refresh; `Ref` gives `ScrollTo`/`ScrollToEnd`.
- List: `OnLoadMore`, `Header`/`Footer`/`Empty`/`Separator`, `Inverted`.
- `ui.List` adds selection (`ui.NewSelection`, `OnActivate`); `ui.ScrollView` adds bars.

**Images, including an in-memory QR PNG** ✔
```go
core.Image{Source: core.Src("https://example.com/cover.jpg"), ResizeMode: core.Cover,
    Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12}}
core.Image{Source: core.ImageSource{Data: pngBytes, Size: core.Size{Width: 200, Height: 200}},
    Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)}}
```
- An image with no size is 0×0 until its style or `Size` sizes it.
- `ImageSource` fields: `URI` (https, file path, `file:`, `data:`, `asset:`), `Data`, `Size`, `Scales`, `Variants`.
- Resize modes: `core.Cover|Contain|Center|ScaleDown`. Callbacks: `OnLoad`/`OnError`. `ui.Image{Source, Label, AspectRatio, Placeholder}` is the styled version.
- QR component: NOT FOUND. `rsc.io/qr` is already an NSGo dependency; the probe uses `code, _ := qr.Encode(text, qr.M); code.Scale = 6; png := code.PNG()`.

**Storage** ✔
```go
store := cx.Storage()
if token, ok := store.GetItem("token"); ok { ... }
err := store.SetItem("token", token)   // *core.StorageFullError past core.StorageQuota (5 MiB)
store.RemoveItem(k); store.Keys(); store.Clear()
cx.DataDir(); cx.CacheDir()            // "" on web
```
The backing store is a plaintext `storage.json` in the data directory (`internal/engine/storage.go`), written atomically. On Android that is the app-private files directory; on Windows `%APPDATA%\<id>`.

**Secure storage (Keychain/Keystore): NOT FOUND.** Bilibili cookies and tokens would sit in plaintext unless you encrypt them yourself.

**Clipboard** ✔
```go
cx.SetClipboard("Hello")
cx.ReadClipboard(func(text string) { pasted.Set(text) })
```
Also `SetClipboardImage([]byte)`, `ReadClipboardImage`, `HasClipboardText`.

**Notifications** ✔
```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
    if p == core.NotificationsGranted {
        cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
    }
})
cx.OnNotificationPress(func(id string) { openChat(id) })
```
- `core.Notification` has only `ID`, `Title`, `Body`, `Silent`; ID and Title are required.
- Also `cx.CloseNotification(id)` and `cx.SetBadge(n)`.
- Android 13+ needs `android: { permissions: [POST_NOTIFICATIONS] }` in `nsgo.yaml`.
- Scheduled or timed local notifications and push: NOT FOUND. Only immediate notifications exist.

**Lifecycle** ✔
```go
cx.Effect(func() { if cx.Env().Lifecycle() == core.LifecycleBackground { ... } })  // LifecycleActive | LifecycleInactive | LifecycleBackground
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })   // 200 ms budget
cx.Quit(); mobile.ExitApp(cx)                             // ExitApp: Android only
```
- `cx.Env()` also has `OS()` (`core.OSIOS`, `core.OSAndroid`, …), `Dark()`, `SafeArea()`, `Locale()`, `Desktop()`.
- Background-task or foreground-service API: NOT FOUND.
- Desktop only: tray icons and `SetShutdownMode(ShutdownOnExplicit)` in package `desktop` (`examples/tray`). I did not compile these.

**Dialogs and toasts** ✔
```go
cx.ShowAlert(core.AlertConfig{Title: "Delete the file?", Message: "This cannot be undone.",
    Actions: []core.AlertAction{{Label: "Cancel", Role: core.AlertCancel}, {Label: "Delete", Role: core.AlertDestructive}},
}, func(index int) { if index == 1 { remove() } })          // Sheet: true gives an action sheet; Cancelable answers -1
ui.Dialog{Open: open.Get, OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
    Title: "Rename", Description: "...", Children: []core.Node{...}, Footer: []core.Node{ui.Button{...}}}
```
- `ui.Dialog` is a bottom sheet on touch screens. Also `ui.Sheet{Open, OnClose, Children}` and `ui.AlertDialog{Open, OnOpenChange, Title, Description, Actions: []ui.AlertAction{{Text, Variant, Color, OnPress}}}`.
- Toasts are not in the docs pages; this is from `ui/toast.go`:
```go
toaster := ui.NewToaster(cx)                               // near the root
ui.Toasts{Toaster: toaster, Placement: ui.ToastBottom}     // place once over the content
toaster.Show(ui.ToastContent{Type: ui.ToastSuccess, Title: "Copied", Description: "", Duration: 2 * time.Second})
```
`ToastType` is `ToastPlain|Info|Success|Warning|Error`. Default duration is 5 s; negative is sticky; `Action` and `OnAction` add a button.

**Theme and dark mode** ✔
```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
cx.SetAppearance(core.AppearanceDark)   // AppearanceLight | AppearanceSystem
cx.Dark(); cx.Palette()
```
- `nsgo.yaml` `appearance: light|dark` fixes the appearance at launch.
- Follow the theme inside components with `StyleFunc: func() core.Style { return core.Style{Color: colors.Text()} }`.
- Tailwind-style `Class: "flex gap-3 dark:bg-zinc-900"` also works.

**i18n**
- Catalogs live in `locales/en.yaml`, `locales/zh.yaml` (ICU MessageFormat, e.g. `greeting: Hello, {name}!`). The `appName` key localises the app name.
- `dev`, `build`, `test` or `go tool nsgo i18n` generate Go functions such as `locales.Greeting(cx, "Ada")`.
- `cx.SetLocale("de")` switches language ✔. `go tool nsgo dev -lang en-XA` runs a pseudo-locale.
- I did not exercise the catalog pipeline, only `SetLocale`.

## (d) Background execution

**NSGo has no background-execution support on Android or iOS.** A search of the docs (`docs/public/en/`, the `application.md`/`services.md`/`embedding.md`/`device-differences.md` architecture pages, `roadmap.md`, `ARCHITECTURE.md`) and of the Go and Java source for foreground services, wake locks, boot receivers, `WorkManager`/`JobScheduler`/`AlarmManager`, `UIBackgroundModes`, `BGTask` and `beginBackgroundTask` found nothing.

**Android**
- **Architecture:** `libnsgo.so` is an ELF PIE executable, not a JNI library. `NsgoView.start()` launches it with `ProcessBuilder` from `nativeLibraryDir` and talks to it over stdin/stdout (`NsgoView.java:460`; `ARCHITECTURE.md` explains why).
- **When backgrounded:** nothing in NSGo stops the Go process. Only window visibility is reported, so `Env.Lifecycle()` becomes background. Goroutines and `net/http` keep running while Android leaves the app process alive.
- **When the activity is destroyed:** `NsgoActivity.onDestroy()` calls `view.stop()`, which calls `process.destroy()`. On the Go side `serve()` reads EOF, closes the engine, runs `OnQuit` handlers and returns.
- **No way to add a service:** the manifest has one activity and a `FileProvider`. `nsgo.yaml`'s `android:` accepts only `permissions` and `target_sdk`. Adding `FOREGROUND_SERVICE` as a permission would do nothing, because the prebuilt `classes.dex` has no `Service` class.
- **Expected OS behaviour [INFERENCE, general Android, not from NSGo sources]:** without a foreground service the app becomes cached, gets frozen or dozed, and may be killed. Android 12+ also has a phantom-process killer aimed at exactly this kind of app child process. A goroutine timer cannot be relied on to fire a daily task.
- **Possible workaround [INFERENCE, not tried]:** `nsgo build -embed` produces an `.aar` for your own Gradle app, whose manifest could declare a foreground service. That needs the Android SDK and Gradle, and the Go process is still owned by an `NsgoView`.

**iOS**
- **Architecture:** Go is linked in-process (iOS forbids child processes). The backend maps `UIApplicationDidEnterBackgroundNotification` to `LifecycleBackground` and closes the engine on `applicationWillTerminate:`.
- **No background modes possible via config:** `apple.info` entries are rendered only as `<string>` values (`cmd/nsgo/build.go:912`), so `UIBackgroundModes`, which is an array, cannot be declared.
- **Expected OS behaviour [INFERENCE]:** the process is suspended shortly after backgrounding and goroutines stop.

**What this means for a daily-task tool:** the mobile apps can run tasks while in the foreground (manual "run now" or catch-up on open) and notify immediately. Unattended scheduling has to live elsewhere: a desktop tray app, a headless Go daemon or server that the mobile app talks to, or a fork of the Android runtime.

## (e) Risks and blockers

1. **Pre-release.** No tags, no stable API ("NSGo is early"), a pseudo-version dependency, and upstream CI triggers are disabled. Pin the pseudo-version.
2. **No background execution on mobile.** This is the main blocker for an unattended scheduler (see (d)).
3. **No secure storage.** `cx.Storage()` is a plaintext JSON file with a 5 MiB quota.
4. **Android limits.**
   - arm64-v8a only with Go alone; x86_64 needs the NDK; there is no 32-bit ARM (`goarchForABI`).
   - `minSdk` is 28 (Android 9+).
   - Signing is v2 only, with RSA keys only.
   - The debug key lives in `.nsgo/android/debug.pem`; losing it breaks updates.
   - The APK carries no app drawables [INFERENCE from `resources.arsc` being 40 bytes], so `TabBarItem.Icon` is probably limited to `android:` system drawables.
5. **iOS needs a Mac.** A CI-built IPA is unsigned without a provisioning profile, so it is not installable on stock devices without re-signing.
6. **Windows installer needs `makensis`.** Without it you get only the exe and the `.nsi` script. A store MSIX is unsigned.
7. **`doctor` is optimistic.** It marks `macos_*` buildable on Windows, but the build fails at `codesign`. `linux_x64` does cross-build from Windows.
8. **Web target is heavy.** The hello-world wasm is 11.3 MB (3.2 MB zipped). [INFERENCE] Browser CORS and cookie rules would also block direct Bilibili API calls from a web build.
9. **Not verified at runtime.** Tab bar rendering, QR display from `ImageSource.Data` and notifications are compile-checked only.
10. **Notifications are immediate only.** There is no scheduling.

## Spike directory contents

Everything is left in place under `.tools/spike-nsgo/`:

- `hello/`: the `init` app with `dist/`, `dist-extra/`, `dist-signed/`, `dist-store/`
- `probe/`: the compile-only API probe and its `dist/`
- `verify-apk/`: the v2 signature verifier
- `keystore/`: a throwaway test key (`release.p12`, password `spikepass`)
- `scratch/`: the extracted `libnsgo.so`
- `nsgo-src/`: a shallow clone of the repo at `70cef6a`, used for docs and source reading