package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/log"
	"github.com/cheshir/ttlcache"
	"github.com/fhs/gompd/v2/mpd"
	"github.com/hugolgst/rich-go/client"
)

const statePlaying = "play"

var (
	shortSleep   = 5 * time.Second
	longSleep    = time.Minute
	songCache    = ttlcache.New(time.Minute)
	artworkCache = ttlcache.New(time.Minute)
	mpdClient    *mpd.Client
	err          error
)

func main() {
	defer func() {
		_ = songCache.Close()
		_ = artworkCache.Close()
	}()

	// Connect to MPD server
	mpdClient, err = mpd.Dial("tcp", "localhost:6600")
	if err != nil {
		log.WithError(err).Fatal("failed to connect to MPD server")
	}
	defer mpdClient.Close()

	if os.Getenv("DARP_DEBUG") != "" {
		log.SetLevelFromString("debug")
	}
	ac := activityConnection{}
	defer func() { ac.stop() }()

	for {
		details, err := getNowPlaying()
		if err != nil {
			log.WithError(err).WithField("sleep", shortSleep).Warn("will try again soon")
			ac.stop()
			time.Sleep(shortSleep)
			continue
		}

		if details.State != statePlaying {
			if ac.connected {
				log.Info("not playing")
				ac.stop()
			}
			time.Sleep(shortSleep)
			continue
		}

		if err := ac.play(details); err != nil {
			log.WithError(err).Warn("could not set activity, will retry later")
		}

		time.Sleep(shortSleep)
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func getNowPlaying() (Details, error) {
	init := time.Now()
	defer func() {
		log.WithField("took", time.Since(init)).Info("got info")
	}()

	status, err := mpdClient.Status()
	if err != nil {
		fmt.Println(err)
		return Details{}, err
	}
	state := status["state"]
	if state != statePlaying {
		return Details{
			State: state,
		}, nil
	}

	initialState, err := mpdClient.CurrentSong()
	if err != nil {
		return Details{}, err
	}

	songID, err := strconv.ParseInt(initialState["Id"], 10, 64)
	if err != nil {
		return Details{}, err
	}

	position, err := strconv.ParseFloat("0", 64)
	if err != nil {
		return Details{}, err
	}

	cached, ok := songCache.Get(ttlcache.Int64Key(songID))
	if ok {
		log.WithField("songID", songID).Debug("got song from cache")
		return Details{
			Song:     cached.(Song),
			Position: position,
			State:    state,
		}, nil
	}

	name := fmt.Sprintf("%s", initialState["Title"])
	artist := fmt.Sprintf("%s", initialState["Artist"])
	album := fmt.Sprintf("%s", initialState["Album"])
	date := fmt.Sprintf("%s", initialState["Date"])
	year, err := strconv.Atoi(strings.Split(date, "-")[0])
	if err != nil {
		return Details{}, err
	}

	url, err := getArtwork(artist, album, name)
	if err != nil {
		return Details{}, err
	}

	song := Song{
		ID:     songID,
		Name:   name,
		Artist: artist,
		Album:  album,
		Year:   year,
		// Duration: duration,
		Artwork: url,
	}

	songCache.Set(ttlcache.Int64Key(songID), song, 24*time.Hour)

	return Details{
		Song:     song,
		Position: position,
		State:    state,
	}, nil
}

type Details struct {
	Song     Song
	Position float64
	State    string
}

type Song struct {
	ID       int64
	Name     string
	Artist   string
	Album    string
	Year     int
	Duration float64
	Artwork  string
}

func getArtwork(artist, album, song string) (string, error) {
	key := url.QueryEscape(strings.Join([]string{artist, album, song}, " "))
	cached, ok := artworkCache.Get(ttlcache.StringKey(key))
	if ok {
		log.WithField("key", key).Debug("got album artwork from cache")
		return cached.(string), nil
	}

	resp, err := http.Get("https://itunes.apple.com/search?term=" + key + "&limit=1&entity=song")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result getArtworkResult
	if err := json.Unmarshal(bts, &result); err != nil {
		return "", err
	}
	if result.ResultCount == 0 {
		return "", nil
	}
	url := result.Results[0].ArtworkUrl100
	artworkCache.Set(ttlcache.StringKey(key), url, time.Hour)
	return url, nil
}

type getArtworkResult struct {
	ResultCount int `json:"resultCount"`
	Results     []struct {
		ArtworkUrl100 string `json:"artworkUrl100"`
	} `json:"results"`
}

type activityConnection struct {
	connected    bool
	lastSongID   int64
	lastPosition float64
}

func (ac *activityConnection) stop() {
	if ac.connected {
		client.Logout()
		ac.connected = false
		ac.lastPosition = 0.0
		ac.lastSongID = 0
	}
}

func (ac *activityConnection) play(details Details) error {
	song := details.Song
	if ac.lastSongID == song.ID {
		if details.Position >= ac.lastPosition {
			log.
				WithField("songID", song.ID).
				WithField("position", details.Position).
				Debug("ongoing activity, ignoring")
			return nil
		}
	}
	log.
		WithField("lastSongID", ac.lastSongID).
		WithField("songID", song.ID).
		WithField("lastPosition", ac.lastPosition).
		WithField("position", details.Position).
		Debug("new event")

	ac.lastPosition = details.Position
	ac.lastSongID = song.ID

	start := time.Now().Add(-1 * time.Duration(details.Position) * time.Second)
	// end := time.Now().Add(time.Duration(song.Duration-details.Position) * time.Second)
	searchURL := fmt.Sprintf("https://music.apple.com/us/search?term=%s", url.QueryEscape(song.Name+" "+song.Artist))
	if !ac.connected {
		if err := client.Login("1037215044141854721"); err != nil {
			log.WithError(err).Fatal("could not create rich presence client")
		}
		ac.connected = true
	}

	if err := client.SetActivity(client.Activity{
		State:      fmt.Sprintf("by %s (%s)", song.Artist, song.Album),
		Details:    song.Name,
		LargeImage: firstNonEmpty(song.Artwork, "applemusic"),
		SmallImage: "play",
		LargeText:  song.Name,
		SmallText:  fmt.Sprintf("%s by %s (%s)", song.Name, song.Artist, song.Album),
		Timestamps: &client.Timestamps{
			Start: timePtr(start),
			// End:   timePtr(end),
		},
		Buttons: []*client.Button{
			{
				Label: "Search on Apple Music",
				Url:   searchURL,
			},
		},
	}); err != nil {
		return err
	}

	log.WithField("song", song.Name).
		WithField("album", song.Album).
		WithField("artist", song.Artist).
		WithField("year", song.Year).
		WithField("duration", time.Duration(song.Duration)*time.Second).
		WithField("position", time.Duration(details.Position)*time.Second).
		Info("now playing")
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
