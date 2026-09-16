from machine import Pin, I2C
import framebuf
import ssd1306
from writer import Writer
import freesans14


def print_spaced(wri, device, s, row, col, spacing=1, invert=False):
    Writer.set_textpos(device, row, col)
    for ch in s:
        wri.printstring(ch, invert)
        state = Writer.state[id(device)]
        state.text_col += spacing


i2c = I2C(scl=Pin(12), sda=Pin(14), freq=400000)
oled = ssd1306.SSD1306_I2C(128, 64, i2c)

wri = Writer(oled, freesans14)
oled.fill(0)
print_spaced(wri, oled, "Hello", 0, 0, spacing=1)
print_spaced(wri, oled, "Nav!", 18, 0, spacing=1)
oled.show()
print("Done")
