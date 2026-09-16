from machine import Pin, I2C

pairs = [(4, 5), (5, 4), (14, 12), (12, 14), (0, 2), (2, 0), (2, 14), (13, 14), (14, 13)]

for scl_pin, sda_pin in pairs:
    try:
        i2c = I2C(scl=Pin(scl_pin), sda=Pin(sda_pin), freq=100000)
        devs = i2c.scan()
        print("scl=%d sda=%d -> %s" % (scl_pin, sda_pin, [hex(a) for a in devs]))
    except Exception as e:
        print("scl=%d sda=%d -> error: %s" % (scl_pin, sda_pin, e))
