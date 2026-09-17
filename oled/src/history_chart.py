# Split out of energy_slides.py (see README's "Gotcha hit and fixed" --
# the same atomic-compile-unit MemoryError applies to any module that both
# imports a sizeable font and defines a lot of its own code): this keeps
# energy_slides.py's own function bodies small so they don't compete with
# its `import berkeley40` for heap.

# Floor for the chart's y-axis span. Below this, an autoscaled
# min(history)/max(history) range zooms in on what's actually just meter
# noise and makes it look like demand is swinging wildly.
MIN_CHART_RANGE_KW = 1.0


def draw(oled, history, x, y, w, h):
    # Autoscaled to the window's actual min/max (not fixed at 0): demand
    # often sits well above zero, and a 0-anchored axis squashes real
    # variation into a thin band near the top -- the chart ends up looking
    # flat even when usage is genuinely swinging. MIN_CHART_RANGE_KW is the
    # floor on that autoscale, so a genuinely flat hour doesn't get zoomed
    # in on meter noise and read as wild swings instead.
    vmin = min(history)
    vmax = max(history)
    if vmax - vmin < MIN_CHART_RANGE_KW:
        vmax = vmin + MIN_CHART_RANGE_KW
    vmid = (vmin + vmax) / 2

    oled.text("{:.1f}".format(vmax), 0, y)
    oled.text("{:.1f}".format(vmid), 0, y + h // 2 - 4)
    oled.text("{:.1f}".format(vmin), 0, y + h - 8)

    n = len(history)
    if n < 2:
        return
    vrange = vmax - vmin or 1

    def point(i):
        px = x + round(i * (w - 1) / (n - 1))
        py = y + h - 1 - round((history[i] - vmin) * (h - 1) / vrange)
        # vmin/vmax are supposed to bound every value in the window, but a
        # meter reading outside that range (a poll landing between one
        # frame's min/max snapshot and the next) shouldn't be able to draw
        # outside the chart's own box.
        py = min(y + h - 1, max(y, py))
        return px, py

    prev = point(0)
    for i in range(1, n):
        cur = point(i)
        oled.line(prev[0], prev[1], cur[0], cur[1], 1)
        prev = cur
