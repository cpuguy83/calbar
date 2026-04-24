# Home Manager module for CalBar
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.calbar;
  yamlFormat = pkgs.formats.yaml { };
in
{
  options.services.calbar = {
    enable = lib.mkEnableOption "CalBar calendar tray app";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.calbar;
      description = "The CalBar package to use.";
    };

    gtk.disable = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Disable GTK and use dmenu-style launchers only. Cannot be used together with a custom package option.";
    };

    settings = lib.mkOption {
      type = yamlFormat.type;
      default = { };
      example = lib.literalExpression ''
        {
          sync = {
            interval = "5m";
          };
          sources = [
            {
              name = "Work";
              type = "ms365";
            }
            {
              name = "Personal";
              type = "ics";
              url = "https://example.com/calendar.ics";
            }
          ];
          notifications = {
            enabled = true;
            before = [ "15m" "5m" ];
          };
        }
      '';
      description = "CalBar configuration. See config.example.yaml for options.";
    };

    css = lib.mkOption {
      type = lib.types.nullOr lib.types.lines;
      default = null;
      example = ''
        .popup-container {
          background: rgba(20, 20, 24, 0.72);
          border: 1px solid rgba(255, 255, 255, 0.08);
        }
      '';
      description = ''
        Optional GTK CSS overrides written to `~/.config/calbar/style.css`.
        This only affects the GTK popup. Users can also set this to `builtins.readFile ./style.css`.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = !(cfg.gtk.disable && cfg.package != pkgs.calbar);
        message = "services.calbar: Cannot set both gtk.disable and a custom package. Use either gtk.disable = true OR package = pkgs.calbar-lite.";
      }
      {
        assertion = !(cfg.css != null && lib.hasAttrByPath [ "ui" "css_file" ] cfg.settings);
        message = "services.calbar: css manages ~/.config/calbar/style.css; do not also set settings.ui.css_file.";
      }
    ];

    home.packages = [
      (if cfg.gtk.disable then pkgs.calbar-lite else cfg.package)
    ];

    xdg.configFile."calbar/config.yaml" = lib.mkIf (cfg.settings != { }) {
      source = yamlFormat.generate "calbar-config.yaml" cfg.settings;
      onChange = "systemctl --user restart calbar.service || true";
    };

    xdg.configFile."calbar/style.css" = lib.mkIf (cfg.css != null) {
      text = cfg.css;
      onChange = "systemctl --user restart calbar.service || true";
    };

    xdg.desktopEntries.calbar = {
      name = "CalBar";
      genericName = "Calendar";
      comment = "Calendar system tray app";
      exec = "${if cfg.gtk.disable then pkgs.calbar-lite else cfg.package}/bin/calbar";
      icon = "x-office-calendar";
      terminal = false;
      type = "Application";
      categories = [
        "Utility"
        "Calendar"
      ]
      ++ lib.optionals (!cfg.gtk.disable) [ "GTK" ];
      startupNotify = false;
    };

    # Optionally auto-start with systemd user service
    systemd.user.services.calbar = lib.mkIf cfg.enable {
      Unit = {
        Description = "CalBar calendar tray app";
        PartOf = [ "graphical-session.target" ];
        After = [ "graphical-session.target" ];
      };
      Service = {
        ExecStart = "${if cfg.gtk.disable then pkgs.calbar-lite else cfg.package}/bin/calbar";
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install = {
        WantedBy = [ "graphical-session.target" ];
      };
    };
  };
}
