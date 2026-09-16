def draw(oled, qr, x, y, scale=1, color=1):
    size = qr.SIZE
    row_bytes = qr.ROW_BYTES
    data = qr.DATA
    target = max(1, round(size * scale))
    for py in range(target):
        src_row = (py * size) // target
        base = src_row * row_bytes
        for px in range(target):
            src_col = (px * size) // target
            byte = data[base + src_col // 8]
            if byte & (0x80 >> (src_col % 8)):
                oled.pixel(x + px, y + py, color)
    return target
