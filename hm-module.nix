# Home Manager module for CalBar
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.calbar;
  tomlFormat = pkgs.formats.yaml { };
in
{
  options.services.calbar = {
    enable = lib.mkEnableOption "CalBar calendar tray app";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.calbar;
      description = "The CalBar package to use.";
    };

    settings = lib.mkOption {
      type = tomlFormat.type;
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
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    xdg.configFile."calbar/config.yaml" = lib.mkIf (cfg.settings != { }) {
      source = tomlFormat.generate "calbar-config.yaml" cfg.settings;
    };

    xdg.desktopEntries.calbar = {
      name = "CalBar";
      genericName = "Calendar";
      comment = "Calendar system tray app";
      exec = "${cfg.package}/bin/calbar";
      icon = "x-office-calendar";
      terminal = false;
      type = "Application";
      categories = [
        "Utility"
        "Calendar"
        "GTK"
      ];
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
        ExecStart = "${cfg.package}/bin/calbar";
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install = {
        WantedBy = [ "graphical-session.target" ];
      };
    };
  };
}
