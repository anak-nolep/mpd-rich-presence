# MPD Discord Rich Presence

Gets your current track from MPD, and shows it in your Discord Rich Presence.

It also gets metadata for the song via Last.fm's API, such as the album cover.
People clicking on your profile can easily view your current track on last.fm.

<p align="center">
  <img src="https://user-images.githubusercontent.com/79978224/199865008-746183c4-f6d5-4e4c-94a0-9f32cfb96eaa.png"/>
</p>

### Installation

#### Linux

Download the binary from the [latest release][release], and execute it.

#### macOS

A Homebrew tap is available:

```bash
nekowinston/tap/mpd-rich-presence
```

Then start & enable the service:

```
brew services start nekowinston/tap/mpd-rich-presence
```

Remember to restart this service after updates.

### Usage

Configuration is available, but not required. The config file can either be
located in `$XDG_CONFIG_HOME`, `~/.config`, or next to the binary.

Example `mpd-rich-presence.yml`, showing the defaults:

```yaml
# MPD connection
host: "127.0.0.1"
port: 6600

# can be "mpd" or "lastfm"
branding: mpd

rich_presence:
  # available keys are:
  # %album%
  # %artist%
  # %title%
  # %year%
  image:
    large: "%album% (%year%)"
    small: "%title%"
  upper: "%title%"
  lower: "by %artist%"
  button: "View on Last.fm"
  # can be "remaining" or "elapsed"
  time: "elapsed"

sleep:
  long: 30s
  short: 5s

# you can turn lastfm off, so no queries will be sent to LastFM.
# Album Art will be empty, it will just show the Logo chosen in "branding"
lastfm:
  enabled: true
  # here you can choose your own api credentials, if you want to
  #apiKey: ""
  #apiSecret: ""
```

### Credits

Forked from [@caarlos0][caarlos0]'s repository:
[Rich Presence from Apple Music][applemusic]

[mpd]: https://github.com/MusicPlayerDaemon/MPD
[release]: https://github.com/nekowinston/mpd-rich-presence/releases/latest
[caarlos0]: https://github.com/caarlos0
[applemusic]: https://github.com/caarlos0/discord-applemusic-rich-presence
