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

      # Build the unwrapped calbar binary
      mkCalbarUnwrapped =
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

          vendorHash = "sha256-AQsa/bkTHAGpZmiQhwfdPB/bWsiWL4SZzKjjOdJRfIY=";

          subPackages = [ "cmd/calbar" ];

          # Add nogtk build tag when GTK is disabled
          tags = pkgs.lib.optionals gtk.disable [ "nogtk" ];

          meta = with pkgs.lib; {
            description = "Calendar system tray app for Linux";
            homepage = "https://github.com/cpuguy83/calbar";
            license = licenses.mit;
            platforms = platforms.linux;
          };
        };

      # Create a combined lib folder with symlinks for puregotk
      # puregotk expects PUREGOTK_LIB_FOLDER to point to a single directory
      mkGtkLibFolder =
        pkgs:
        let
          libs = [
            pkgs.gtk4
            pkgs.gtk4-layer-shell
            pkgs.libadwaita
            pkgs.glib.out # Explicit .out for libraries
            pkgs.cairo
            pkgs.pango.out # Explicit .out for libraries
            pkgs.gdk-pixbuf
            pkgs.graphene
          ];
        in
        pkgs.runCommand "calbar-gtk-libs" { } ''
          mkdir -p $out/lib
          for lib in ${pkgs.lib.concatMapStringsSep " " (l: "${l}/lib") libs}; do
            for f in $lib/*.so*; do
              if [ -f "$f" ] && [ ! -e "$out/lib/$(basename $f)" ]; then
                ln -s "$f" "$out/lib/$(basename $f)"
              fi
            done
          done
        '';

      # Wrap calbar with GTK library paths for NixOS
      # puregotk loads libraries via dlopen and needs PUREGOTK_LIB_FOLDER
      mkCalbarWrapped =
        pkgs:
        let
          unwrapped = mkCalbarUnwrapped pkgs { };
          gtkLibs = mkGtkLibFolder pkgs;
        in
        pkgs.runCommand "calbar-${unwrapped.version}"
          {
            nativeBuildInputs = [ pkgs.makeWrapper ];
            meta = unwrapped.meta // {
              mainProgram = "calbar";
            };
          }
          ''
            mkdir -p $out/bin
            makeWrapper ${unwrapped}/bin/calbar $out/bin/calbar \
              --set PUREGOTK_LIB_FOLDER "${gtkLibs}/lib"
          '';
    in
    {
      packages = forAllSystems (
        { pkgs }:
        {
          default = mkCalbarWrapped pkgs;
          calbar = mkCalbarWrapped pkgs;
          calbar-unwrapped = mkCalbarUnwrapped pkgs { };
          calbar-lite = mkCalbarUnwrapped pkgs { gtk.disable = true; };
        }
      );

      overlays.default = final: prev: {
        calbar = mkCalbarWrapped final;
        calbar-unwrapped = mkCalbarUnwrapped final { };
        calbar-lite = mkCalbarUnwrapped final { gtk.disable = true; };
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
