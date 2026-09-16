from machine import Pin, I2C
import framebuf
import ssd1306


def text_scaled(oled, s, x, y, scale=2, color=1):
    w, h = 8 * len(s), 8
    buf = bytearray(w * h // 8)
    fb = framebuf.FrameBuffer(buf, w, h, framebuf.MONO_VLSB)
    fb.text(s, 0, 0, 1)
    for yy in range(h):
        for xx in range(w):
            if fb.pixel(xx, yy):
                oled.fill_rect(x + xx * scale, y + yy * scale, scale, scale, color)


i2c = I2C(scl=Pin(12), sda=Pin(14), freq=400000)
print("I2C devices:", [hex(a) for a in i2c.scan()])

oled = ssd1306.SSD1306_I2C(128, 64, i2c)
oled.fill(0)
text_scaled(oled, "Hello, Nav!", 0, 0, scale=2)
text_scaled(oled, "ESP8266", 0, 32, scale=2)
oled.show()
print("Done")
