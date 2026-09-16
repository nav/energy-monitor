import gc
import time
from writer import Writer
import freesansbold40
import power_api

# Same 128x64 dual-color split documented in the README (top 16 rows
# yellow, bottom 48 blue) -- duplicated here rather than imported from
# main.py to avoid a circular import (main.py imports this module).
BLUE_ZONE_TOP = 16
BODY_H = 64 - BLUE_ZONE_TOP

HISTORY_POINTS = 60  # one point/minute -> last hour
POLL_INTERVAL_MS = 60_000
SLIDE_INTERVAL_MS = 10_000


# freesansbold40 is digit/period-only (see "Generating a different font
# size" in README) -- every digit glyph in it is exactly 29px wide and '.'
# is 13px, so a string's rendered width is fully predictable: 29*ndigits +
# 13*ndots. format_big() relies on that to guarantee at most 3 digits
# (<=100px), which always fits centered on the 128px-wide screen.
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
    # freesans14 (regular weight) instead of the built-in bitmap font --
    # the built-in 8x8 font renders every pixel solid black/white with no
    # antialiasing, which at this size reads as much bolder/blockier than
    # an actual bold font. freesans14 is thinner and more legible.
    Writer.set_textpos(oled, 0, 0)
    wri.printstring(title)


def _draw_big_number(oled, wri, wri_big, title, value_str):
    oled.fill(0)
    _draw_title(oled, wri, title)

    w = sum(freesansbold40.get_ch(c)[2] for c in value_str)
    x = max(0, (128 - w) // 2)
    y = BLUE_ZONE_TOP + (BODY_H - freesansbold40.height()) // 2
    Writer.set_textpos(oled, y, x)
    wri_big.printstring(value_str)
    oled.show()


def _draw_line_chart(oled, values, x, y, w, h, vmin):
    n = len(values)
    if n < 2:
        return
    vmax = max(values)
    vrange = vmax - vmin or 1

    def point(i):
        px = x + round(i * (w - 1) / (n - 1))
        py = y + h - 1 - round((values[i] - vmin) * (h - 1) / vrange)
        return px, py

    prev = point(0)
    for i in range(1, n):
        cur = point(i)
        oled.line(prev[0], prev[1], cur[0], cur[1], 1)
        prev = cur


def _show_history_slide(oled, wri, history):
    oled.fill(0)
    _draw_title(oled, wri, "Demand (kW)")

    # No x-axis tick labels -- the window is always exactly the last hour,
    # so that row is reclaimed for chart height instead.
    label_w = 34  # fits "{:.1f}" labels (e.g. "23.4") at 8px/char
    x, y = label_w, BLUE_ZONE_TOP + 1
    w, h = 128 - label_w - 2, BODY_H - 2

    # Fixed at 0 rather than min(history): demand doesn't go negative here,
    # and an autoscaled floor exaggerates small fluctuations into a chart
    # that looks like it's swinging wildly when it's actually +/- a few %.
    vmin = 0
    vmax = max(history)
    oled.text("{:.1f}".format(vmax), 0, y)
    oled.text("{:.1f}".format(vmin), 0, y + h - 8)
    _draw_line_chart(oled, history, x, y, w, h, vmin)
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
    oled.fill(0)
    _draw_title(oled, wri, "Fetching data...")
    oled.show()

    wri_big = Writer(oled, freesansbold40)

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
        time.sleep_ms(200)
