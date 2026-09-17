import gc
import time
from micropython import const
from writer import Writer
import berkeley40
import history_chart
import power_api

# Same 128x64 dual-color split documented in the README (top 16 rows
# yellow, bottom 48 blue) -- duplicated here rather than imported from
# main.py to avoid a circular import (main.py imports this module).
BLUE_ZONE_TOP = 16
BODY_H = 64 - BLUE_ZONE_TOP

HISTORY_POINTS = 60  # one point/minute -> last hour
POLL_INTERVAL_MS = 60_000
SLIDE_INTERVAL_MS = 10_000

# Power saving: sit dim most of the time, briefly go bright on a timer so the
# screen is still readable at a glance without staying at full brightness
# (and full OLED current draw) continuously.
#
# contrast() (0-255) alone can't get dim enough on this panel: 0 cuts the
# segment drive current entirely (pure black, no light regardless of any
# other setting), but 1 -- the smallest nonzero step -- is still clearly lit.
# There's no finer step in between. SET_PRECHARGE (0xD9, not exposed by
# ssd1306.py -- poked directly, same as main.py's show_region()) is a second,
# independent knob: it sets how long each row's pixels get to charge before
# being driven, so a short precharge visibly dims contrast=1 well below its
# default-precharge brightness. DIM_PRECHARGE is a starting guess -- tune it
# by eye (lower phase-2 nibble = dimmer; 0x11 is close to the register's
# floor, 0xF1 is the driver's own init-time default).
SET_PRECHARGE = const(0xD9)
DIM_CONTRAST = 1  # lowest nonzero contrast step
DIM_PRECHARGE = 0x22
BRIGHT_CONTRAST = round(255 * 0.80)
BRIGHT_PRECHARGE = 0xF1  # ssd1306.SSD1306.init_display()'s internal-VCC default
BRIGHT_INTERVAL_MS = 5 * 60_000
BRIGHT_DURATION_MS = 30_000


def _set_dim(oled):
    oled.write_cmd(SET_PRECHARGE)
    oled.write_cmd(DIM_PRECHARGE)
    oled.contrast(DIM_CONTRAST)


def _set_bright(oled):
    oled.write_cmd(SET_PRECHARGE)
    oled.write_cmd(BRIGHT_PRECHARGE)
    oled.contrast(BRIGHT_CONTRAST)


# berkeley40 is digit/period-only (see "Generating a different font size"
# in README) -- BerkeleyMono is a true monospace face, so every glyph in
# it, including '.', is exactly 25px wide: a string's rendered width is
# always 25*len(s). format_big() relies on that to guarantee at most 3
# characters (<=75px), which always fits centered on the 128px-wide screen.
def format_big(value):
    if value >= 100:
        s = str(round(value))
    elif value >= 10:
        s = "{:.1f}".format(value)
    else:
        s = "{:.2f}".format(value)
    if sum(c.isdigit() for c in s) > 3:  # float rounded up across a boundary
        s = str(round(value))
    return s


def _draw_title(oled, wri, title):
    # berkeley (regular weight) instead of the built-in bitmap font --
    # the built-in 8x8 font renders every pixel solid black/white with no
    # antialiasing, which at this size reads as much bolder/blockier than
    # an actual bold font. berkeley is thinner and more legible.
    Writer.set_textpos(oled, 0, 0)
    wri.printstring(title)


def _draw_big_number(oled, wri, wri_big, title, value_str):
    oled.fill(0)
    _draw_title(oled, wri, title)

    w = sum(berkeley40.get_ch(c)[2] for c in value_str)
    x = max(0, (128 - w) // 2)
    y = BLUE_ZONE_TOP + (BODY_H - berkeley40.height()) // 2
    Writer.set_textpos(oled, y, x)
    wri_big.printstring(value_str)
    oled.show()


def _show_history_slide(oled, wri, history):
    oled.fill(0)
    _draw_title(oled, wri, "Demand (kW)")

    # No x-axis tick labels -- the window is always exactly the last hour,
    # so that row is reclaimed for chart height instead.
    #
    # label_w budgets for a 5-char "{:.1f}" label ("-12.4", "123.4"), not
    # just the usual 4 ("23.4"): the EAGLE's demand reading is a signed
    # value (see eagle.go), so a solar-export reading or a meter-resync
    # glitch can legitimately land outside the usual 1-2-digit range. A
    # label that overflows this column bleeds into the chart's own
    # x-range -- right at the row the line's low point sits on, which
    # reads as the line itself spilling past the chart's bottom edge.
    label_w = 42  # 5 chars * 8px, plus the same 2px margin the old 4-char budget had
    x, y = label_w, BLUE_ZONE_TOP + 1
    w, h = 128 - label_w - 2, BODY_H - 2
    history_chart.draw(oled, history, x, y, w, h)
    oled.show()


def _poll(history, latest):
    try:
        data = power_api.fetch_current()
        latest["kw"] = data["kw"]
        latest["cost"] = data["cost_dollars"]
        history.pop(0)
        history.append(data["kw"])
    except Exception as e:
        print("power_api fetch failed:", e)  # keep showing last-known values
    gc.collect()


def run(oled, wri):
    _set_dim(oled)
    oled.fill(0)
    _draw_title(oled, wri, "Fetching data...")
    oled.show()

    wri_big = Writer(oled, berkeley40)

    latest = {"kw": 0.0, "cost": 0.0}
    history = [0.0] * HISTORY_POINTS
    _poll(history, latest)  # get a real first reading before the first draw
    # Seed the whole hour with that reading instead of leaving it fronted by
    # zeros -- otherwise the chart shows a misleading flatline-then-spike
    # until the buffer naturally fills an hour from now.
    history[:] = [latest["kw"]] * HISTORY_POINTS

    slides = [
        lambda: _draw_big_number(oled, wri, wri_big, "Demand (kW)", format_big(latest["kw"])),
        lambda: _show_history_slide(oled, wri, history),
        lambda: _draw_big_number(oled, wri, wri_big, "Cost/Hr ($)", format_big(latest["cost"])),
    ]
    idx = 0
    last_poll = time.ticks_ms()
    last_slide = time.ticks_ms()
    last_bright = time.ticks_ms()  # start of the current dim/bright period
    bright = False
    slides[idx]()

    while True:
        now = time.ticks_ms()
        if time.ticks_diff(now, last_poll) >= POLL_INTERVAL_MS:
            _poll(history, latest)
            last_poll = now
            slides[idx]()  # refresh whatever's on screen with the new reading
        if time.ticks_diff(now, last_slide) >= SLIDE_INTERVAL_MS:
            idx = (idx + 1) % len(slides)
            last_slide = now
            slides[idx]()
        if not bright and time.ticks_diff(now, last_bright) >= BRIGHT_INTERVAL_MS:
            _set_bright(oled)
            bright = True
            last_bright = now
        elif bright and time.ticks_diff(now, last_bright) >= BRIGHT_DURATION_MS:
            _set_dim(oled)
            bright = False
            last_bright = now
        time.sleep_ms(200)
