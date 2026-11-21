<p align="center">
  <img src="build/appicon.svg" alt="malachi logo" width="128">
</p>

<h1 align="center">malachi-desktop</h1>

<p align="center">
  A desktop app for the <a href="https://github.com/ddombrow/malachi">malachi</a> coding agent, with malachi built in.<br>
  Built with Wails 3 (Go + Svelte 5).
</p>

malachi is built in: the app imports it as a Go module (pinned in `go.mod`)
and runs malachi's own RPC server in-process, over in-memory pipes, for the
project you open. The Go side (`malachiservice.go`) writes commands and
matches responses by id; every other record is emitted to the webview as a
`malachi:event`, exactly as a client of `malachi -mode rpc` would see them.
All of the agent's rules — one run at a time, steering, the sandbox, project
trust — stay in malachi. No separate malachi install is needed; provider
keys are read from `~/.malachi/.env` as the CLI does.

On Linux, malachi's sandbox runs each command by re-launching the current
binary in a helper mode, so `main` hands control to
`sandbox.MaybeRunHelper()` before anything else, as malachi's own `main`
does. To pick up a newer malachi, bump it in `go.mod` and rebuild.

```text
main.go                 sandbox helper hook, Wails app + window
malachiservice.go       runs malachi's RPC server in-process, lists sessions
settings.go             the app's own model/thinking preferences
platform.go             macOS login-shell PATH, app data directory
frontend/src/App.svelte layout: sidebar, transcript, composer, status bar
frontend/src/lib/
  session.svelte.ts     client-side session model fed by RPC events
  ToolCard.svelte       one tool call: summary, args, diff, output
  markdown.ts           marked + DOMPurify (model output is untrusted)
```

## Run

```sh
go install -tags gtk3 github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0   # once
wails3 dev      # hot-reload dev mode
wails3 build    # bin/malachi-desktop
```

The app's version is `info.version` in `build/config.yml`; it shows at the
bottom of the sidebar with malachi's. After bumping it, run
`wails3 task common:update:build-assets` so the platform metadata follows.

On Linux, `wails3 task install:desktop` builds and installs the app to
`~/.local/bin` with its `.desktop` file and icons (`uninstall:desktop`
removes them). Wayland docks only show the app's icon once that is
installed: they match the window's app id, `io.github.ddombrow.malachi-desktop`,
to an installed `.desktop` file.

`wails3 task linux:package` builds an AppImage and `.deb`/`.rpm`/Arch
packages into `bin/` (or one at a time: `linux:create:deb` and so on). They
are defined by `build/linux/nfpm/malachi.yaml` (Wails' generated `nfpm.yaml`
is unused) and install the same app id, desktop file and icons as
`install:desktop`, with the version from `build/config.yml`.

On Linux the build uses GTK3 + webkit2gtk-4.1 (`EXTRA_TAGS` defaults to
`gtk3` in `Taskfile.yml`); install `webkitgtk-6.0` and pass
`EXTRA_TAGS=nogtk3` to use Wails' default GTK4 backend instead.

## Using it

The app opens straight into a chat with no project. malachi always needs a
working directory, so these chats run in an empty one of the app's own,
`$XDG_DATA_HOME/malachi-desktop/chat` (default `~/.local/share/…`; on macOS
`~/Library/Application Support/malachi-desktop/chat`), and are
listed in the sidebar like any project's sessions. **Open project…** in the
sidebar switches to a project, continuing its latest session; ✕ goes back to
chatting.

**Recent projects** in the sidebar are read from malachi's own session
history (`~/.malachi/sessions/*/…jsonl`, whose first record names its `cwd`),
so projects you used from the TUI show up too. Temporary directories,
directories inside a hidden one, the chat directory, and directories that no
longer exist are left out.

- **Enter** sends; while the agent works, Enter steers it, **Alt+Enter**
  queues a follow-up, **Esc** or ■ stops it.
- Slash commands: `/new`, `/compact [note]`, `/model [provider/]id`,
  `/thinking level`, `/network on|off`, `/allow-read dir`, `/allow-write dir`,
  `/trust yes|no`.
- The sidebar lists this project's saved sessions (click to switch),
  background jobs and MCP servers. The status bar holds the model and
  thinking pickers, the sandbox network toggle, the context meter, and
  malachi's stderr log.

## macOS

The window's title bar is hidden and the page runs under it; the sidebar
and main area start below the window buttons. An app started from the Dock
or Finder gets macOS's bare default `PATH`, so at startup the app adopts your
login shell's `PATH` (`$SHELL -i -l`), letting malachi's commands find
Homebrew, Go, Node and the rest. Builds need a Mac with Xcode's command-line
tools; unsigned builds are blocked by Gatekeeper until signed and notarized.

## Settings

The model, thinking level and code mode you pick in the status bar are
remembered in
`$XDG_CONFIG_HOME/malachi-desktop/settings.json` (default `~/.config/…`; on
macOS `~/Library/Application Support/…`) and
applied to every session the app starts or switches to (like the CLI's
`-model`/`-thinking` flags, they win over the model a resumed session used). malachi's own `~/.malachi/settings.json` is
never written; it still supplies the default until you pick something here.

## Icon

`build/make_logo.py` draws the emblem (`build/logo.svg`) and the app icon
(`build/appicon.svg`) from exact geometry. After changing it, regenerate the
rest:

```sh
python3 build/make_logo.py build
rsvg-convert -w 1024 -h 1024 build/appicon.svg -o build/appicon.png
rsvg-convert -w 256 -h 256 build/appicon.svg -o frontend/public/appicon.png
rsvg-convert -w 256 -h 256 build/appicon.svg -o build/linux/icons/256x256.png
rsvg-convert -w 512 -h 512 build/appicon.svg -o build/linux/icons/512x512.png
wails3 task common:generate:icons    # Windows .ico, macOS .icns
```

## License

MIT — see [LICENSE](LICENSE). Bundles the Inter font under the SIL Open Font License (`frontend/Inter Font License.txt`).
