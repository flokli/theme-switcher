# theme-switcher

This configures kitty and helix to honor the GNOME-wide color scheme (Dark mode
or not).

For this to work, it needs `pkill` and `gsettings` in `$PATH`.

## Home-Manager config:

```nix
  systemd.user.services.theme-switcher = {
    Service = {
      Type = "simple";
      ExecStart = "${theme-switcher}/bin/theme-switcher";
    };
    Install.WantedBy = [ "default.target" ];
  };
```
