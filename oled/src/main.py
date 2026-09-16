from machine import Pin, I2C
import framebuf
import time
import ssd1306
from writer import Writer
import freesans14
import qr_draw
import qr_wifi
import qr_url
import checkmark_anim
import wifi_manager
import energy_slides

i2c = I2C(scl=Pin(12), sda=Pin(14), freq=400000)
oled = ssd1306.SSD1306_I2C(128, 64, i2c)
wri = Writer(oled, freesans14)

# Onboard FLASH button on GPIO0, pulled low when pressed. Only safe to read
# once MicroPython is already running (GPIO0 low *during* power-on/reset
# instead puts the ROM bootloader into flash-download mode) — so this is
# checked early in main(), never during the reset edge itself.
RESET_BUTTON = Pin(0, Pin.IN, Pin.PULL_UP)
RESET_ARM_MS = 5000  # window to notice the prompt and start pressing
RESET_HOLD_MS = 3000  # then keep holding this long, measured from first press
RESET_MIN_DUTY = 0.6  # tolerate contact bounce/flicker while the button is held


def show(lines):
    oled.fill(0)
    Writer.set_textpos(oled, 0, 0)
    wri.printstring("\n".join(lines))
    oled.show()


BLUE_ZONE_TOP = 16  # top 16 rows are the yellow strip on this dual-color OLED
BODY_H = 64 - BLUE_ZONE_TOP
LEFT_COL_W = round(128 * 0.2)  # step-number column
QR_SCALE = 1.5
QR_MARGIN = 2  # gap between the right-aligned QR code and the screen edge


def draw_step_circle(oled, cx, cy, step):
    digit = str(step)
    oled.text(digit, cx - 4 * len(digit), cy - 4)
    oled.ellipse(cx, cy, 8, 8, 1)


def show_wizard_screen(step, qr_module):
    oled.fill(0)
    oled.text("Setup Wi-Fi", 0, 0)  # title bar, yellow strip

    left_cx = LEFT_COL_W // 2
    left_cy = BLUE_ZONE_TOP + BODY_H // 2
    draw_step_circle(oled, left_cx, left_cy, step)

    qr_size = round(qr_module.SIZE * QR_SCALE)
    qr_x = 128 - qr_size - QR_MARGIN
    qr_y = BLUE_ZONE_TOP + (BODY_H - qr_size) // 2
    qr_draw.draw(oled, qr_module, qr_x, qr_y, scale=QR_SCALE)

    oled.show()


def show_wifi_qr_screen():
    show_wizard_screen(1, qr_wifi)


def show_url_qr_screen():
    show_wizard_screen(2, qr_url)


def show_region(oled, x0, y0, x1, y1):
    # Partial-window I2C update: pushing only the animation's bounding box
    # instead of the full 1024-byte framebuffer cuts a ~70ms show() to
    # ~25ms on this display, which is most of the animation's frame budget.
    page0 = y0 // 8
    page1 = y1 // 8
    oled.write_cmd(0x21)
    oled.write_cmd(x0)
    oled.write_cmd(x1)
    oled.write_cmd(0x22)
    oled.write_cmd(page0)
    oled.write_cmd(page1)
    width = x1 - x0 + 1
    for page in range(page0, page1 + 1):
        start = page * 128 + x0
        oled.write_data(oled.buffer[start : start + width])


def show_connected_animation():
    x = (128 - checkmark_anim.WIDTH) // 2
    y = BLUE_ZONE_TOP + (BODY_H - checkmark_anim.HEIGHT) // 2
    x1, y1 = x + checkmark_anim.WIDTH - 1, y + checkmark_anim.HEIGHT - 1
    # Rounded out to whole pages/columns for the partial I2C update below.
    ry0 = y // 8 * 8
    ry1 = ((y1 // 8) + 1) * 8 - 1
    rw, rh = x1 - x + 1, ry1 - ry0 + 1

    oled.fill(0)
    oled.show()

    frame_bytes = checkmark_anim.ROW_BYTES * checkmark_anim.HEIGHT
    with open(checkmark_anim.DATA_FILE, "rb") as f:
        for i in range(checkmark_anim.FRAME_COUNT):
            t0 = time.ticks_ms()
            # MONO_HLSB matches our packing exactly (row-major, MSB first),
            # so framebuf.blit() converts it in C instead of a Python pixel
            # loop -- ~17x faster (4.7ms vs 79.5ms measured on this board).
            data = bytearray(f.read(frame_bytes))
            frame = framebuf.FrameBuffer(data, checkmark_anim.WIDTH, checkmark_anim.HEIGHT, framebuf.MONO_HLSB)
            oled.fill_rect(x, ry0, rw, rh, 0)
            oled.blit(frame, x, y)
            show_region(oled, x, ry0, x1, ry1)
            remaining = checkmark_anim.DURATIONS[i] - time.ticks_diff(time.ticks_ms(), t0)
            if remaining > 0:
                time.sleep_ms(remaining)

    time.sleep_ms(500)


def run_setup_mode():
    wifi_manager.start_ap()
    show_wifi_qr_screen()
    # Once a phone joins the AP (having scanned the WiFi QR), switch to
    # showing the URL QR so the same scan gesture opens the config page.
    wifi_manager.run_portal(on_client_connected=show_url_qr_screen)


def check_factory_reset():
    # Always gives a window right after boot, cued by this message, rather
    # than sampling once at the instant main() starts: GPIO0 can't reliably
    # be read as "already pressed" that early (and holding it low during the
    # actual reset pulse instead drops the ROM into flash-download mode), so
    # the user needs an on-screen cue and time to react to it.
    #
    # Split into two phases so reaction time doesn't eat into the hold
    # measurement: first just wait (unmeasured) for any press to arrive,
    # then measure duty cycle over a fresh window starting from that press.
    show(["Starting..."])
    arm_start = time.ticks_ms()
    while RESET_BUTTON.value() != 0:
        if time.ticks_diff(time.ticks_ms(), arm_start) >= RESET_ARM_MS:
            print("factory reset: no press detected within {}ms".format(RESET_ARM_MS))
            return False
        time.sleep_ms(20)

    start = time.ticks_ms()
    samples = 0
    pressed = 0
    while time.ticks_diff(time.ticks_ms(), start) < RESET_HOLD_MS:
        samples += 1
        if RESET_BUTTON.value() == 0:
            pressed += 1
        time.sleep_ms(50)
    duty = pressed / samples
    print("factory reset button: {}/{} samples low ({:.0%})".format(pressed, samples, duty))
    return duty >= RESET_MIN_DUTY


def main():
    if check_factory_reset():
        print("factory reset triggered - clearing wifi.json")
        wifi_manager.reset_config()
        show(["Factory reset", "complete"])
        time.sleep(1)
        run_setup_mode()
        return

    config = wifi_manager.load_config()
    if config:
        show(["Connecting to:", config["ssid"]])
        ip = wifi_manager.connect_sta(config["ssid"], config["password"])
        if ip:
            show_connected_animation()
            energy_slides.run(oled, wri)  # never returns; this is the device's home screen

    run_setup_mode()


main()
