{
  description = "CalBar - Calendar system tray app for Linux";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems =
        f:
        nixpkgs.lib.genAttrs systems (
          system:
          f {
            pkgs = nixpkgs.legacyPackages.${system};
          }
        );
    in
    {
      packages = forAllSystems (
        { pkgs }:
        {
          default = pkgs.buildGoModule {
            pname = "calbar";
            version = "0.1.0";
            src = ./.;

            vendorHash = "sha256-A7lstv7WrFpn6wTQV6xPj83Yf7ctstq2ah5xwK15m9c=";

            subPackages = [ "cmd/calbar" ];

            nativeBuildInputs = [ pkgs.pkg-config ];
            buildInputs = [
              pkgs.gtk4
              pkgs.gtk4-layer-shell
              pkgs.glib
              pkgs.gobject-introspection
            ];

            meta = with pkgs.lib; {
              description = "Calendar system tray app for Linux";
              homepage = "https://github.com/cpuguy83/calbar";
              license = licenses.mit;
              platforms = platforms.linux;
            };
          };
        }
      );

      devShells = forAllSystems (
        { pkgs }:
        {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
              go
              gopls
              pkg-config
              gtk4
              gtk4-layer-shell
              glib
              gobject-introspection
            ];
          };
        }
      );
    };
}
