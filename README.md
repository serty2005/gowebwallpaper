# Go Web Wallpaper

Go Web Wallpaper keeps a WebView2 page in a strict borderless fullscreen window on a selected Windows monitor. The window is forced to `topmost` and periodically repaired if Windows or another app moves/resizes it.

## Current Behavior

- Runs on Windows 10/11.
- Creates a local `config.json` on first start.
- Lets you select the target monitor from the tray menu.
- Keeps the WebView2 window borderless, fullscreen, and topmost.
- Re-checks the selected monitor and repairs the window position every second.
- Lets you select an audio output from the tray menu.
- Injects an audio probe into the page:
  - unmutes `<audio>` and `<video>` elements;
  - reports playback events to the app log;
  - tries to route media elements with `HTMLMediaElement.setSinkId()` when the browser exposes matching output devices.

Audio routing is best-effort. WebView2 does not expose a simple native "set output device" API through the current Go wrapper, and pages using WebAudio may ignore `setSinkId()`. In that case Windows' default output is used.

## Build

```powershell
go test ./...
go build -o gowebwallpaper.exe .
```

To build without a console window:

```powershell
go build -ldflags="-H windowsgui" -o gowebwallpaper.exe .
```

## Run

```powershell
.\gowebwallpaper.exe
```

On first run the app scans connected monitors and creates `config.json` next to the executable. During development it uses the repository `config.json` when that file already exists.

## Logs

The app writes runtime logs to `gowebwallpaper.log` next to `gowebwallpaper.exe`. The log records startup, tray actions, monitor search attempts, ambiguous monitor matches, window repair, restarts, audio selection, and audio playback probe events.

## Tray Menu

- `Start / Restart`: restarts the WebView2 window.
- `Monitor`: selects the exact target monitor.
- `Audio output`: selects the desired output device or `System default`.
- `Exit`: stops the window and exits the tray app.

## Config

Use `config.example.json` as a template. `config.json` is local machine state and is ignored by git.

```json
{
  "URL": "http://localhost:3100/#/columns-fullscreen",
  "Monitors": [
    {
      "Name": "\\\\.\\DISPLAY2",
      "IsPrimary": false,
      "Active": true,
      "PositionX": -2160,
      "PositionY": -395,
      "Width": 1080,
      "Height": 1920
    }
  ],
  "Audio": {
    "ID": "",
    "Name": "",
    "Active": false
  }
}
```

Monitor matching prefers exact `Name`, then exact bounds, then a unique size-only fallback. If two monitors have the same size and neither name nor bounds match, the app refuses the ambiguous match instead of opening on the wrong screen.

## Verification

```powershell
gofmt -w (Get-ChildItem -Filter *.go | ForEach-Object { $_.FullName })
go test ./...
go vet ./...
go build -o gowebwallpaper.exe .
```
