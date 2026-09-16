import socket
import json

HOST = "10.1.50.5"
PORT = 31397
PATH = "/current"


def fetch_current(timeout=5):
    # Hand-rolled GET instead of urequests: the response is ~200 bytes of
    # JSON, and avoiding the extra library keeps this in line with how
    # wifi_manager.py talks HTTP elsewhere in this project (see README).
    # "Connection: close" on the request lets us just read until the
    # socket closes rather than parsing Content-Length ourselves.
    addr = socket.getaddrinfo(HOST, PORT)[0][-1]
    s = socket.socket()
    s.settimeout(timeout)
    try:
        s.connect(addr)
        req = "GET {} HTTP/1.1\r\nHost: {}\r\nConnection: close\r\n\r\n".format(PATH, HOST)
        s.send(req.encode())
        data = b""
        while True:
            chunk = s.recv(512)
            if not chunk:
                break
            data += chunk
    finally:
        s.close()

    sep = data.find(b"\r\n\r\n")
    if sep == -1:
        raise ValueError("malformed response from power API")
    return json.loads(data[sep + 4 :])
