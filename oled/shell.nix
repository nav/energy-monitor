{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = [
    pkgs.esptool
    pkgs.mpremote
    pkgs.freefont_ttf
    (pkgs.python3.withPackages (ps: [ ps.freetype-py ps.qrcode ps.pillow ]))
  ];
}
