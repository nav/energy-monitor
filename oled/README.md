# ESP8266 OLED

Board: [ESP8266 NodeMCU + 0.96" dual-color OLED](https://www.amazon.ca/Development-Wireless-Micropython-Programming-Soldered/dp/B0BW55X6GS) (ESP-12E, CH340 USB driver), running MicroPython.

## Hardware notes

- Serial port: `/dev/cu.usbserial-210`
- OLED is I2C at address `0x3c`, wired to **SCL=GPIO12, SDA=GPIO14** (not the usual GPIO4/5 — verified with `src/i2c_scan.py`)
- Display resolution: 128x64, **dual-color**: top 16 rows render yellow, bottom 48 rows render blue (there's no separate hardware for this — it's just physically two LED strips behind one monochrome panel, so it's still driven as one normal 128x64 SSD1306 framebuffer; you just have to plan layout around the color split)
- Onboard voltage regulator is an **AMS1117** (confirmed by inspection) — typically draws ~5mA continuously whenever the board is powered via USB/VIN, regardless of the ESP8266's own sleep state. Relevant for battery work (see below). No always-on power LED was observed on this board.
- GPIO16 (D0) is exposed (standard NodeMCU pinout) — needed later to wire a D0–RST jumper for deep-sleep wake.

## Setup

All tools (`esptool`, `mpremote`, `python3`+`freetype-py`+`qrcode`, FreeSans font) are provided via Nix:

```bash
nix-shell
```

Run this once per terminal session before using any command below, or prefix each command with `nix-shell --run "..."`.

A `Makefile` wraps the common operations below (`make deploy`, `make run`, `make repl`, `make reset`, `make ls`, `make font ...`, `make flash-firmware CONFIRM=1`) — it auto-detects `/dev/cu.usbserial-*` (override with `PORT=...`) and already wraps everything in `nix-shell --run` for you. Run `make help` for the full list. The rest of this doc spells out what those targets actually do.

## Editing and running code

Scripts live in `src/`. Edit a file, then run it directly on the board without copying it first:

```bash
mpremote connect /dev/cu.usbserial-210 run src/main.py
```

This streams the script's `print()` output back to your terminal live — the fastest loop for iterating. **Important**: any `mpremote ... exec "..."` or `... run ...` command sends a Ctrl-C to the board first, which will kill whatever's currently running (e.g. it'll stop the WiFi portal server mid-flight). Don't run diagnostic `exec` commands while trying to test something that needs to keep running — use one long-lived `run` session and just watch its output instead. Also note: simply *opening* the serial port (any tool, including a plain `cat`) toggles DTR and hard-resets the board — there's no way to pop in and passively observe without resetting it.

**`mpremote run` uses more heap than a real boot, and will falsely report `MemoryError` near the memory ceiling.** `run` pipes the script's source over serial and compiles it from that stream; a real boot compiles `main.py` straight from flash, which needs less peak memory for the same file. When chasing a memory-margin bug like the one below, `mpremote run` can fail while the exact same file, actually deployed (`make push FILE=...` or `make deploy`) and boot-tested with a real reset, succeeds every time. Don't trust a `run`-based repro of a memory error without also confirming it against a real deployed boot.

### Files that must live on the board itself

Everything main.py depends on needs to be copied onto the board's filesystem (not just run from your machine):

```bash
mpremote connect /dev/cu.usbserial-210 fs cp src/main.py :main.py
mpremote connect /dev/cu.usbserial-210 fs cp src/wifi_manager.py :wifi_manager.py
mpremote connect /dev/cu.usbserial-210 fs cp src/writer.py :writer.py
mpremote connect /dev/cu.usbserial-210 fs cp src/berkeley.py :berkeley.py
mpremote connect /dev/cu.usbserial-210 fs cp src/qr_draw.py :qr_draw.py
mpremote connect /dev/cu.usbserial-210 fs cp src/qr_wifi.py :qr_wifi.py
mpremote connect /dev/cu.usbserial-210 fs cp src/qr_url.py :qr_url.py
mpremote connect /dev/cu.usbserial-210 fs cp src/berkeley40.py :berkeley40.py
mpremote connect /dev/cu.usbserial-210 fs cp src/power_api.py :power_api.py
mpremote connect /dev/cu.usbserial-210 fs cp src/energy_slides.py :energy_slides.py
mpremote connect /dev/cu.usbserial-210 fs cp src/history_chart.py :history_chart.py
mpremote connect /dev/cu.usbserial-210 fs cp src/checkmark_anim.py :checkmark_anim.py
mpremote connect /dev/cu.usbserial-210 fs cp src/checkmark_anim.bin :checkmark_anim.bin
```

(`make deploy` does all of the above, plus a `reset` at the end.)

Re-run the relevant line any time you change that file. List what's currently on the board with:

```bash
mpremote connect /dev/cu.usbserial-210 fs ls
```

`main.py` on the board auto-runs on every power-up/reset (MicroPython convention). To force a fresh run after copying files:

```bash
mpremote connect /dev/cu.usbserial-210 reset
```

### Interactive REPL

```bash
mpremote connect /dev/cu.usbserial-210
```

Ctrl-] to exit. (Remember: connecting resets the board.)

## WiFi manager (AP setup + station mode)

`src/wifi_manager.py` + `src/main.py` implement a self-service WiFi onboarding flow, no hardcoded credentials needed:

1. On boot, `main.py` checks for `wifi.json` on the board (via `wifi_manager.load_config()`).
2. **If found**: tries `wifi_manager.connect_sta(ssid, password, timeout=15)`. On success, shows the IP on the OLED. On failure, falls through to setup mode.
3. **If not found (or connect failed)**: enters setup mode —
   - `wifi_manager.start_ap()` brings up an **open** AP named `ESP8266-Setup` at `192.168.4.1` (explicitly disables STA first — see gotcha below).
   - OLED shows the AP SSID in the yellow strip, and a **WiFi-join QR code** (`WIFI:T:nopass;S:ESP8266-Setup;;`) in the blue zone — scanning it with a phone camera joins the network directly, no typing/manual WiFi-settings needed.
   - `wifi_manager.run_portal()` runs a minimal hand-rolled HTTP server (no external framework — MicroPython has no `http.server`) on port 80, polling `ap.status("stations")` once a second. As soon as a phone joins the AP, the OLED **automatically switches** to a second QR code encoding `http://192.168.4.1`, so the same scan gesture that joined the network can now open the config page.
   - The page is a plain SSID/password form (no network-scan dropdown — see gotcha below). POST `/save` writes `wifi.json` and calls `machine.reset()`, which re-enters this flow and now succeeds via the stored credentials.

### Gotchas hit and fixed (don't reintroduce these)

- **MicroPython `bytes` has no `.partition()`** (that's CPython-only) — use `.find()` + slicing instead. This caused every portal request to silently 500 until fixed.
- **HTTP responses need explicit `Content-Length` and `Connection: close`** — without them, mobile browsers report "network connection was lost" because they can't tell a clean end-of-response from a dropped socket.
- **Read the full request before parsing** — a single `recv()` call can return a partial request (headers arrive before the POST body on a slow AP link). `_recv_request()` accumulates until `\r\n\r\n`, then reads exactly `Content-Length` more bytes.
- **Accepted sockets inherit the listening socket's `settimeout()`** — since the AP-detection loop needs `srv.settimeout(1)` to poll periodically, every accepted connection was *also* getting a 1s timeout, causing real (slightly slow) requests to fail with `ETIMEDOUT`. Fix: call `conn.settimeout(5)` right after `accept()`.
- **ESP8266's WiFi SDK auto-reconnects STA using previously-saved credentials the instant the STA interface is activated** — completely independent of what's in `wifi.json`. We originally scanned for nearby networks (to populate a `<datalist>` in the form) by flipping STA on, which silently triggered this auto-reconnect and destabilized the AP (ESP8266 can't run AP+STA on independent channels — STA forces the AP onto its own channel). **Fix applied: dropped the network-scan feature entirely** and made `start_ap()` explicitly `sta.disconnect(); sta.active(False)` before bringing up the AP, so STA is never touched during setup mode.
- **The board kept advertising the setup AP (`Energy Monitor`) even after successfully connecting to WiFi.** Cause: the ESP8266 WiFi SDK persists its interface mode (STA/AP/STA+AP) in flash across reboots, independent of this code — if the board last rebooted out of setup mode (AP active, e.g. mid-portal), the SDK brings the AP back up on the next boot before `main.py`'s own logic runs, and nothing in the normal `wifi.json`-found path ever turned it back off. **Fix applied**: `connect_sta()` now explicitly calls `network.WLAN(network.AP_IF).active(False)` before activating STA, rather than assuming the AP is already off.
- Debugging note: any diagnostic `mpremote exec` you run to "just check state" will interrupt whatever's currently running on the board (see note above) — this cost significant time when a perfectly-working portal was mistaken for broken because checking on it killed it.

### Factory reset

Every boot, before touching WiFi, `main.py` shows `Hold FLASH for factory reset...` on the OLED and watches the onboard **FLASH button (GPIO0)** for 3 seconds. Press and hold it during that window (only *after* the message appears — not during the actual power-on/reset pulse, since GPIO0 low at that instant puts the ROM bootloader into flash-download mode instead of booting MicroPython) and it wipes `wifi.json` via `wifi_manager.reset_config()`, then drops straight into setup mode (`run_setup_mode()`, no reboot needed). Not holding it just costs a fixed 3s boot delay and then continues normally.

The check tolerates contact bounce — `check_factory_reset()` requires the pin to read low for at least 60% of samples across the window (`RESET_MIN_DUTY`), not a perfectly clean signal, since the onboard button flickers briefly even while held (confirmed by watching `Pin(0).value()` live).

**Gotcha**: an earlier version of this sampled the button once at the instant `main()` started, before anything was shown on screen — nearly un-hittable, since there's no cue to react to and a human can't hold the button through the reset pulse itself (that triggers flash mode instead of booting). Always give the button-press window a visible on-screen cue and let it span a real duration.

Note: this adds a fixed 3s delay to *every* boot, which matters once the battery-powered deep-sleep mode below is built (it wakes every 5 minutes) — revisit or skip this check in that code path when it's written.

## QR codes

Both QR codes are **pre-generated on the dev machine** (via the `qrcode` Python package, added to `shell.nix`) and embedded as static pixel bitmaps — the ESP8266 never runs a QR encoder itself, it just draws pixels. This is deliberate: writing a real QR encoder (Reed-Solomon ECC, mask selection, etc.) is a lot of code for an 80KB-RAM chip, and both payloads (`WIFI:T:nopass;S:ESP8266-Setup;;` and `http://192.168.4.1`) are 100% static.

- `src/qr_draw.py` — generic nearest-neighbor-scaling pixel drawer, takes any `{SIZE, ROW_BYTES, DATA}` module and a `scale` (supports fractional scale, e.g. `1.5`, not just integer block-doubling)
- `src/qr_wifi.py`, `src/qr_url.py` — generated bitmap data (25×25 modules each, QR version 2, error-correction level L)

To regenerate (e.g. if the AP SSID or IP ever changes), see the inline generator pattern used to create these — a short Python script using `qrcode.QRCode(...).get_matrix()`, packed 1-bit-per-module row-major into a `bytes` literal. Ask to regenerate rather than hand-editing the data.

Layout (`main.py`'s `show_qr_screen()`): SSID/label text in the yellow strip (`oled.text()`, plain 8×8 font, y=0), QR code at `scale=1.5` (≈38×38px) centered in the blue zone, optional hint text below it.

## Energy slides (home screen)

Once WiFi connects, `energy_slides.run(oled, wri)` (called from `main.py`, reusing its existing `berkeley` `Writer` for titles rather than importing a second copy of that font) becomes the permanent home screen (it never returns). It cycles three slides every 10s (`SLIDE_INTERVAL_MS`):

1. **Demand (kW)** — current draw, big bold centered number.
2. **Demand history** — line graph of the last hour (drawn by `src/history_chart.py`, split out of `energy_slides.py` — see gotcha below), filling the full body height; y-axis autoscaled to the window's actual min/max (not fixed at 0 — demand often sits well above zero, and a 0-anchored axis squashes real variation into a thin band near the top), labeled with the window's min, midpoint, and max. `MIN_CHART_RANGE_KW` floors the autoscale range so a genuinely flat hour doesn't get zoomed in on meter noise and read as wild swings. The EAGLE's demand reading is a signed value (see `eagle.go`), so the label column is sized for a 5-character reading (`-12.4`, `123.4`), not just the usual 2-digit one — a label that overflowed its column used to bleed into the chart itself, right at the line's low point, and looked like the line spilling past the chart's own bottom edge. No x-axis tick labels — the window is always exactly the last hour, so that's implicit rather than spelled out.
3. **Cost/Hr ($)** — same big-number layout as slide 1.

Titles use `berkeley` (regular weight) via `writer.py`'s `Writer`, not the built-in `oled.text()` bitmap font — the built-in 8x8 font has no antialiasing, so at this size it reads noticeably bolder/blockier than actual bold text.

To cut power draw, the screen sits at the SSD1306's lowest contrast setting, 0 (`DIM_CONTRAST`), and briefly steps up to 80% (`BRIGHT_CONTRAST`) for 30s every 5 minutes (`BRIGHT_INTERVAL_MS`/`BRIGHT_DURATION_MS`), via `oled.contrast()`. This only affects `energy_slides.py`'s home-screen loop — the setup wizard, QR screens, and connected-animation run at the driver's default full contrast.

Data comes from `src/power_api.py`, which does a hand-rolled raw-socket `GET` (no `urequests` — keeps the same zero-dependency approach as `wifi_manager.py`'s server side) against a fixed local endpoint (`HOST`/`PORT`/`PATH` constants at the top of the file), polled once a minute (`POLL_INTERVAL_MS`). Each poll appends to a 60-point ring buffer (`history`, one point/minute = 1 hour) that feeds the history slide; a failed poll just logs and keeps showing the last-known values rather than crashing the loop.

The big-number font (`src/berkeley40.py`) is BerkeleyMono Bold UltraCondensed at 40px, generated with a **digit-only charset** (`-c '0123456789.'`) — see "Generating a different font size" below. Restricting the charset like this instead of the full ASCII set keeps a 40px font's *source* to ~9KB instead of ~18KB+ (though the resident cost of any of these fonts is much smaller than the source file size suggests — see gotcha below). BerkeleyMono is a true monospace face, so every glyph, including `.`, comes out exactly 25px wide, which `format_big()` in `energy_slides.py` relies on to guarantee output never exceeds 3 characters (75px) — comfortably inside the 128px screen width, with more headroom than the FreeSansBold font it replaced (29px/digit, tight against a 129px 4-digit overflow).

### Gotcha hit and fixed: `MemoryError` importing a font from `main.py`, but not standalone

Adding the slides directly into `main.py` (as more top-level functions + two font imports) reliably crashed with `MemoryError: memory allocation failed, allocating 1865 bytes` on `import freesansbold40` — even though `gc.mem_free()` measured right before that import showed 20KB+ free, and the same import succeeded fine when tried in isolation over the REPL.

Cause: MicroPython compiles an entire `.py` file into bytecode as one atomic unit before executing any of it — so a ~300-line `main.py` defining 15+ functions leaves a lot of small, permanently-live bytecode objects scattered through the heap *before* any of its top-level imports even run. `gc.collect()` between imports doesn't help (that memory isn't garbage, it's still-referenced code). The subsequent big font import then needs one contiguous multi-KB block for its bytes literal, and couldn't find one despite plenty of free space in aggregate — classic non-compacting-allocator fragmentation, not an out-of-memory condition.

**Fix applied**: moved the new slide code into its own module (`energy_slides.py`) instead of adding it to `main.py`. `main.py` now compiles as a small file again, and the font import happens during `energy_slides.py`'s own (separate, smaller) compile pass instead of competing with a much larger one. Confirmed fixed by running the whole thing live on the board.

**Lesson for next time**: if a future addition to this device needs another sizeable data/font import, give it its own module rather than growing `main.py` — don't wait for a `MemoryError` to notice `main.py` has become the thing you're importing everything alongside.

**Hit again, same cause, different lever**: swapping `freesans14` (9.9KB, `main.py`'s general-text font) for a `BerkeleyMono` font generated at height 23 (21.5KB, full ASCII charset) reproduced this exact `MemoryError` on `import berkeley` inside `main.py`. Unlike `freesansbold40`, this font needs the full charset (it renders arbitrary SSIDs and status text, not just digits), so the charset-restriction fix didn't apply — the lever here was height instead. Font byte size doesn't scale smoothly with height: glyph rows are packed at 1 byte per 8px of width, so any height whose glyphs cross an 8px-width boundary roughly doubles in size. For this font that boundary sits between 18px (safe, ~9.9KB) and 20px (~19.9KB, still fits but with much less headroom) — 23px (~21.5KB) is the one that finally overflows. Regenerated at height 18 to land back in the same safe size class as the font it replaced.

**Hit a third time, this time from ordinary code growth, not a font**: fixing the history chart's y-axis (autoscaling instead of a fixed 0 floor, then adding a third label and a bounds clamp — a few lines, no new imports) was enough on its own to flip `import berkeley40` inside `energy_slides.py` from passing 4/4 real-boot trials to failing 4/4. Nothing about the font changed; `energy_slides.py`'s own function bodies just got a little bigger, which is exactly the same atomic-compile-unit pressure as the first two incidents, just from code instead of data. The margin here is thin enough that a 3-line diff can flip it either way — don't assume a module is safe just because it was previously measured passing. **Fix applied**: split the chart-drawing code out into its own module (`src/history_chart.py`), the same move as the original `energy_slides.py` split from `main.py`, restoring headroom by keeping `energy_slides.py` itself small again.

## Legible text (general display work)

For anything *besides* the QR/status screens — the default `ssd1306.text()` font is a fixed 8×8 bitmap; scaling it up just makes the same blocky shape bigger, doesn't fix legibility. For real legible text we use Peter Hinch's `writer.py` framework with a font generated from FreeSans.ttf at a specific pixel height (not the built-in font), since it's a properly-designed font at that size rather than a scaled bitmap.

- `src/writer.py` — the `Writer` framework (downloaded from [peterhinch/micropython-font-to-py](https://github.com/peterhinch/micropython-font-to-py))
- `src/berkeley.py` — Berkeley Mono generated at 18px, see "Generating a different font size" below
- `src/freesans20.py` — generated FreeSans font at 20px, unused but kept as a reference example
- `src/oled_writer_test.py` — demonstrates `Writer` usage plus a manual letter-spacing helper (`Writer` has no built-in tracking/spacing control)

## Generating a different font size

Fonts are generated from the Berkeley Mono UltraCondensed TTFs in `oled/assets/` using `tools_font_to_py.py`. `assets/` has every weight (Thin through Black) in both upright and Oblique — pick the file matching the weight you want:

```bash
python3 tools_font_to_py.py assets/BerkeleyMono-Light-UltraCondensed.ttf <height_px> --fixed src/berkeley.py
```

Then update the `import berkeley` line in your script and copy the new font file to the board as above. `--fixed` forces a uniform glyph width — worth keeping for predictable layout math, though BerkeleyMono is naturally monospaced already so it rarely changes the output.

For a large font that's only ever going to render a handful of characters (digits for a numeric readout, say), restrict the charset with `-c`: `python3 tools_font_to_py.py assets/BerkeleyMono-Bold-UltraCondensed.ttf 40 --fixed -c '0123456789.' src/foo.py` generates only those glyphs, which is the difference between a 40px font costing ~9KB vs ~18KB+ for the full ASCII set — worth doing any time you don't need every character. See `src/berkeley40.py` (used by the energy slides, below) for an example.

**Watch the memory ceiling, not just file size**: this board has hit real `MemoryError`s from font imports before (see the gotchas below) — always confirm a new/regenerated font with a real device reboot (not `mpremote run`, which uses more heap than a real boot and can report a false failure — see the caveat above), not just by checking the generated file's byte count.

## Re-flashing MicroPython

Firmware binary is in `firmware/esp8266-firmware.bin` (MicroPython v1.29.0). To reflash from scratch:

```bash
esptool --port /dev/cu.usbserial-210 erase-flash
esptool --port /dev/cu.usbserial-210 --baud 460800 write-flash --flash-size=detect 0x0 firmware/esp8266-firmware.bin
```

## Other files

- `src/oled_test.py`, `src/i2c_scan.py` — early scratch scripts from initial OLED bring-up (I2C pin discovery, basic `ssd1306.text()` smoke test). Not part of the current app but harmless to keep as reference.
- `tools_font_to_py.py` — the font generator tool itself (downloaded from the same repo as `writer.py`)

## In progress / not yet implemented: battery-powered periodic JSON fetch

Next planned direction (discussed but not yet built): move away from always-on WiFi manager operation toward an independent, battery-powered device that wakes every 5 minutes, fetches a JSON API, optionally shows something on the OLED, and goes back to deep sleep — running off a single 18650 cell.

Key facts established so far:
- **Deep sleep wake requires a hardware jumper**: wire GPIO16 (D0) to RST. `machine.deepsleep(ms)` then causes a full reboot after the timeout (deep sleep on ESP8266 doesn't preserve RAM except a small RTC memory area) — each wake re-runs `main.py` from scratch, so the wake routine needs to reconnect WiFi, fetch, then sleep again.
- **The onboard AMS1117 regulator draws ~5mA continuously** any time the board is powered via USB/VIN, regardless of what the ESP8266 chip itself is doing (deep sleep drops the ESP8266 itself to ~20µA, but that's meaningless if a 5mA regulator sits inline the whole time). At 5mA continuous, even a 3000mAh 18650 would drain in ~3-4 weeks.
- **The fix doesn't require desoldering anything**: never power the board's USB/VIN input at all. Instead feed a separate low-quiescent-current 3.3V regulator (e.g. MCP1700-3302E, ~1.6µA quiescent, needs just two small caps, ~$1) directly into the board's **3V3 pin** from the battery (optionally via a TP4056 charge/protection module for rechargeability). With nothing feeding AMS1117's input, it draws nothing.
- No always-on power LED was found on this board (checked by inspection), so that's one less thing to worry about that affects some NodeMCU clones.

Not yet done: the external regulator/battery wiring (hardware, on the user), the D0-RST jumper (hardware), or the deep-sleep fetch-loop software (`main.py` rewrite for this mode — straightforward once the above is confirmed working: connect STA using saved `wifi.json`, `urequests.get(...)` the JSON API, optionally draw to OLED, `machine.deepsleep(5 * 60 * 1000)`).

Also discussed and set aside: MQTT (`umqtt.simple`/`umqtt.robust`, both available in this firmware) would be the efficient choice if the design instead needed the *external server* to push messages to the board on its own schedule; ESP-NOW (also available) would be the choice for direct ESP-to-ESP messaging without a router. Neither is needed for the current polling-based direction.
