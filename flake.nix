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

      mkCalbar =
        pkgs:
        {
          gtk ? {
            disable = false;
          },
        }:
        pkgs.buildGoModule {
          pname = "calbar";
          version = "0.1.0";
          src = ./.;

          vendorHash = "sha256-Rh2Y0TR95gt67uxo7QRFHZq0pwu6BSaniAaD1FuxZ8E=";

          subPackages = [ "cmd/calbar" ];

          # Add nogtk build tag when GTK is disabled
          tags = pkgs.lib.optionals gtk.disable [ "nogtk" ];

          doCheck = false; # Tests require D-Bus/GTK

          nativeBuildInputs = pkgs.lib.optionals (!gtk.disable) [ pkgs.pkg-config ];
          buildInputs = pkgs.lib.optionals (!gtk.disable) [
            pkgs.gtk4
            pkgs.gtk4-layer-shell
            pkgs.libadwaita
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
    in
    {
      packages = forAllSystems (
        { pkgs }:
        {
          default = mkCalbar pkgs { };
          calbar = mkCalbar pkgs { };
          calbar-lite = mkCalbar pkgs { gtk.disable = true; };
        }
      );

      overlays.default = final: prev: {
        calbar = mkCalbar final { };
        calbar-lite = mkCalbar final { gtk.disable = true; };
      };

      homeManagerModules.default = import ./hm-module.nix;

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
              libadwaita
              glib
              gobject-introspection
            ];
          };
        }
      );
    };
}
