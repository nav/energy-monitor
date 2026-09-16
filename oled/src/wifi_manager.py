import gc
import json
import machine
import network
import os
import socket
import time

CONFIG_FILE = "wifi.json"
AP_SSID = "Energy Monitor"
AP_IP = "192.168.4.1"


def load_config():
    try:
        with open(CONFIG_FILE) as f:
            return json.load(f)
    except OSError:
        return None


def save_config(ssid, password):
    with open(CONFIG_FILE, "w") as f:
        json.dump({"ssid": ssid, "password": password}, f)


def reset_config():
    try:
        os.remove(CONFIG_FILE)
    except OSError:
        pass


def connect_sta(ssid, password, timeout=15):
    sta = network.WLAN(network.STA_IF)
    sta.active(True)
    sta.connect(ssid, password)

    deadline = time.time() + timeout
    while not sta.isconnected() and time.time() < deadline:
        time.sleep(0.5)

    if sta.isconnected():
        return sta.ifconfig()[0]

    sta.active(False)
    return None


def start_ap(ssid=AP_SSID):
    sta = network.WLAN(network.STA_IF)
    try:
        sta.disconnect()
    except OSError:
        pass
    sta.active(False)

    ap = network.WLAN(network.AP_IF)
    ap.active(True)
    ap.config(essid=ssid, authmode=network.AUTH_OPEN)

    deadline = time.time() + 5
    while not ap.active() and time.time() < deadline:
        time.sleep(0.1)

    return ap.ifconfig()[0]


def _unquote(s):
    s = s.replace("+", " ")
    res = ""
    i = 0
    while i < len(s):
        if s[i] == "%" and i + 2 < len(s):
            res += chr(int(s[i + 1 : i + 3], 16))
            i += 3
        else:
            res += s[i]
            i += 1
    return res


def _parse_form(body):
    params = {}
    for pair in body.split("&"):
        if "=" in pair:
            k, v = pair.split("=", 1)
            params[_unquote(k)] = _unquote(v)
    return params


def _render_form():
    return """<!DOCTYPE html>
<html><head><title>WiFi Setup</title>
<meta name="viewport" content="width=device-width, initial-scale=1"></head>
<body>
<h2>WiFi Setup</h2>
<form method="POST" action="/save">
<label>Network</label><br>
<input name="ssid" placeholder="SSID" required><br>
<label>Password</label><br>
<input type="password" name="password"><br><br>
<button type="submit">Save &amp; Connect</button>
</form>
</body></html>"""


def _recv_request(conn):
    data = b""
    while b"\r\n\r\n" not in data:
        chunk = conn.recv(1024)
        if not chunk:
            break
        data += chunk

    sep = data.find(b"\r\n\r\n")
    if sep == -1:
        header_part, rest = data, b""
    else:
        header_part, rest = data[:sep], data[sep + 4 :]
    headers = header_part.decode()
    content_length = 0
    for line in headers.split("\r\n")[1:]:
        if line.lower().startswith("content-length:"):
            content_length = int(line.split(":", 1)[1].strip())

    body = rest
    while len(body) < content_length:
        chunk = conn.recv(1024)
        if not chunk:
            break
        body += chunk

    request_line = headers.split("\r\n", 1)[0]
    return request_line, body.decode()


def _send_response(conn, status, body):
    body_bytes = body.encode()
    header = (
        "HTTP/1.1 {}\r\n"
        "Content-Type: text/html\r\n"
        "Content-Length: {}\r\n"
        "Connection: close\r\n\r\n"
    ).format(status, len(body_bytes))
    conn.send(header.encode() + body_bytes)


def run_portal(on_client_connected=None):
    form_html = _render_form()
    ap = network.WLAN(network.AP_IF)

    addr = socket.getaddrinfo("0.0.0.0", 80)[0][-1]
    srv = socket.socket()
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind(addr)
    srv.listen(1)
    srv.settimeout(1)

    notified = on_client_connected is None
    while True:
        stations = ap.status("stations")
        if not notified and stations:
            notified = True
            on_client_connected()

        try:
            conn, _ = srv.accept()
        except OSError:
            continue  # accept() timed out; loop back to poll stations again

        conn.settimeout(5)  # accepted sockets inherit the listener's 1s timeout
        try:
            request_line, body = _recv_request(conn)
            if not request_line:
                continue
            method, path, _ = request_line.split(" ")

            if method == "POST" and path == "/save":
                params = _parse_form(body)
                ssid = params.get("ssid", "")
                password = params.get("password", "")
                if ssid:
                    save_config(ssid, password)
                    _send_response(conn, "200 OK", "<html><body><h3>Saved. Rebooting...</h3></body></html>")
                    conn.close()
                    time.sleep(1)
                    machine.reset()
                else:
                    _send_response(conn, "400 Bad Request", "<html><body>Missing SSID</body></html>")
            else:
                _send_response(conn, "200 OK", form_html)
        except Exception as e:
            print("portal error:", e)
        finally:
            conn.close()
            gc.collect()
