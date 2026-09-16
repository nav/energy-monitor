# Common operations for the ESP8266 OLED firmware. Everything runs through
# `nix-shell --run "..."`, per README.md, so no local mpremote/esptool
# install is needed -- just Nix.

PORT ?= $(shell ls -t /dev/cu.usbserial-* 2>/dev/null | head -1)

# Files main.py transitively imports -- must live on the board itself, not
# just in src/ on this machine. Keep in sync with README.md's file list.
FILES := main.py wifi_manager.py writer.py freesans14.py qr_draw.py \
         qr_wifi.py qr_url.py freesansbold40.py power_api.py \
         energy_slides.py checkmark_anim.py checkmark_anim.bin

DEPLOY_CMD := $(foreach f,$(FILES),mpremote connect $(PORT) fs cp src/$(f) :$(f) &&) mpremote connect $(PORT) reset

.PHONY: help deploy push run repl reset ls flash-firmware font check-port

help:
	@echo "make deploy                                          - copy all board-resident files, then reset"
	@echo "make push FILE=<name>                                - copy a single file (e.g. FILE=energy_slides.py)"
	@echo "make run                                              - stream src/main.py live (mpremote run)"
	@echo "make repl                                             - open an interactive REPL"
	@echo "make reset                                            - reset the board"
	@echo "make ls                                               - list files on the board"
	@echo "make flash-firmware CONFIRM=1                         - erase flash and reflash MicroPython (destructive)"
	@echo "make font HEIGHT=<px> OUT=<path> [CHARSET=<chars>]    - generate a Writer font from FreeSans.ttf"
	@echo ""
	@echo "PORT defaults to the most recently attached /dev/cu.usbserial-* device (currently: $(PORT))."
	@echo "Override with PORT=/dev/cu.usbserial-XXX."

check-port:
	@if [ -z "$(PORT)" ]; then \
		echo "No /dev/cu.usbserial-* device found. Plug in the board, or set PORT explicitly:"; \
		echo "  make <target> PORT=/dev/cu.usbserial-XXX"; \
		exit 1; \
	fi

deploy: check-port
	nix-shell --run "$(DEPLOY_CMD)"

push: check-port
	@if [ -z "$(FILE)" ]; then echo "Usage: make push FILE=<name.py>"; exit 1; fi
	nix-shell --run "mpremote connect $(PORT) fs cp src/$(FILE) :$(FILE)"

run: check-port
	nix-shell --run "mpremote connect $(PORT) run src/main.py"

repl: check-port
	nix-shell --run "mpremote connect $(PORT)"

reset: check-port
	nix-shell --run "mpremote connect $(PORT) reset"

ls: check-port
	nix-shell --run "mpremote connect $(PORT) fs ls"

flash-firmware: check-port
	@if [ "$(CONFIRM)" != "1" ]; then \
		echo "This erases the entire board (including wifi.json) and reflashes MicroPython."; \
		echo "Re-run as: make flash-firmware CONFIRM=1"; \
		exit 1; \
	fi
	nix-shell --run "esptool --port $(PORT) erase-flash"
	nix-shell --run "esptool --port $(PORT) --baud 460800 write-flash --flash-size=detect 0x0 firmware/esp8266-firmware.bin"

font:
	@if [ -z "$(HEIGHT)" ] || [ -z "$(OUT)" ]; then \
		echo "Usage: make font HEIGHT=<px> OUT=src/freesansNN.py [CHARSET=<chars>]"; \
		exit 1; \
	fi
	nix-shell --run '\
		FONT_PATH=$$(find /nix/store -maxdepth 1 -iname "*freefont-ttf*" ! -name "*.drv" | head -1)/share/fonts/truetype/FreeSans.ttf; \
		python3 tools_font_to_py.py "$$FONT_PATH" $(HEIGHT) $(OUT) $(if $(CHARSET),-c "$(CHARSET)",) \
	'
