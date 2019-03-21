package main

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	runewidth "github.com/mattn/go-runewidth"
)

const delimiter string = "::::"

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func failOn(b bool, message string) {
	if b {
		fail(errors.New(message))
	}
}

type Stack []string

func (s *Stack) Push(dirname string) {
	*s = append(*s, dirname)
}

func (s *Stack) DiscardTop() {
	stack := *s
	failOn(len(stack) <= 0, "Invalid directory state. Corrupted database?")
	i := len(stack) - 1
	*s = stack[:i]
}

func keyval(line string) (string, string) {
	i := strings.Index(line, ":")
	if i == -1 || i == len(line)-1 {
		return line, ""
	}
	return line[:i], line[i+2:]
}

type Track struct {
	Album    string
	Artist   string
	Date     string
	Filename string
	Genre    string
	Path     string
	Time     string
	Title    string
}

func (t *Track) Set(key, value string) {
	switch key {
	case "Album":
		t.Album = value
	case "Artist":
		t.Artist = value
	case "Date":
		t.Date = value
	case "Genre":
		t.Genre = value
	case "Time":
		t.Time = formatDurationString(value)
	case "Title":
		t.Title = value
	}
}

func formatDurationString(str string) string {
	duration, err := time.ParseDuration(str + "s")
	if err != nil {
		return ""
	}
	zero := time.Time{}
	format := zero.Add(duration).Format("04:05")
	if duration > time.Hour {
		format = fmt.Sprintf("%d:%s", int(duration.Hours()), format)
	}
	return "(" + format + ")"
}

func withoutExt(path string) string {
	basename := filepath.Base(path)
	return strings.TrimSuffix(basename, filepath.Ext(basename))
}

func alignLeftRight(maxlen int, description, duration string) string {
	stop := maxlen - len(duration)
	s := runewidth.Truncate(description, stop, "... ")
	return runewidth.FillRight(s, stop) + duration
}

func termWidth() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	fail(err)
	var height, width int
	_, err = fmt.Sscanf(string(out), "%d %d\n", &height, &width)
	fail(err)
	return width
}

func trackFormatter() func(*Track) string {
	// Remove 5 from screen width for correct fzf display at right edge.
	width := termWidth() - 5
	return func(t *Track) string {
		info := t.Title
		if info == "" {
			info = withoutExt(t.Filename)
		}
		if t.Artist != "" {
			info = t.Artist + " - " + info
		}
		if t.Album != "" {
			info += " {" + t.Album + "}"
		}
		info = strings.Replace(info, delimiter, "", -1)
		// Right align duration
		info = alignLeftRight(width, info, t.Time)
		return info + delimiter + t.Path
	}
}

func groupByArtist(tracks []*Track) []*Track {
	// group by artist, then shuffle to stop same order, but keep artist together
	artists := map[string][]*Track{}
	for _, t := range tracks {
		artists[t.Artist] = append(artists[t.Artist], t)
	}
	shuffled := make([]*Track, len(tracks))
	i := 0
	for _, tracks := range artists {
		for _, t := range tracks {
			shuffled[i] = t
			i += 1
		}
	}
	return shuffled
}

func parse(scan *bufio.Scanner) []*Track {

	tracks, track := []*Track{}, new(Track)
	dirstack := make(Stack, 0, 64)

	for scan.Scan() {
		key, value := keyval(scan.Text())
		switch key {
		case "directory":
			dirstack.Push(value)
		case "end":
			dirstack.DiscardTop()
		case "Artist", "Album", "Date", "Genre", "Time", "Title":
			track.Set(key, value)
		case "song_begin":
			track.Filename = value
			track.Path = filepath.Join(append(dirstack, track.Filename)...)
		case "song_end":
			tracks = append(tracks, track)
			track = new(Track)
		}
	}
	fail(scan.Err())
	return tracks
}

func expandUser(path, home string) string {
	if path[:2] == "~/" {
		path = strings.Replace(path, "~", home, 1)
	}
	return path
}

func findDbFile() string {
	usr, err := user.Current()
	fail(err)
	home := usr.HomeDir
	paths := []string{
		filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "/mpd/mpd.conf"),
		filepath.Join(home, ".config", "/mpd/mpd.conf"),
		filepath.Join(home, ".mpdconf"),
		"/etc/mpd.conf",
	}
	var f *os.File
	var confpath string
	for _, path := range paths {
		f, err = os.Open(path)
		if err == nil {
			confpath = path
			break
		}
	}
	failOn(f == nil, "No config file found")

	expDb := regexp.MustCompile(`^\s*db_file\s*"([^"]+)"`)
	scan := bufio.NewScanner(f)
	var dbFile string
	for scan.Scan() {
		m := expDb.FindStringSubmatch(scan.Text())
		if m != nil {
			dbFile = expandUser(m[1], home)
		}
	}
	fail(scan.Err())
	fail(f.Close())
	failOn(dbFile == "", fmt.Sprintf("Could not find 'db_file' in configuration file '%s'", confpath))
	return dbFile
}

func fzfcmd() *exec.Cmd {
	bind := "--bind=enter:execute-silent(mpd-fzf-play {})"
	fzf := exec.Command("fzf", "--no-hscroll", bind)
	fzf.Stderr = os.Stderr
	return fzf
}

func ignoreExitInterrupt(err error) error {
	if strings.HasSuffix(err.Error(), "130") {
		return nil
	}
	return err
}

func main() {
	dbFile := findDbFile()
	format := trackFormatter()

	f, err := os.Open(dbFile)
	fail(err)
	gz, err := gzip.NewReader(f)
	fail(err)

	scan := bufio.NewScanner(gz)
	tracks := groupByArtist(parse(scan))

	fail(gz.Close())
	fail(f.Close())

	fzf := fzfcmd()
	in, err := fzf.StdinPipe()
	fail(err)
	fail(fzf.Start())
	for _, t := range tracks {
		fmt.Fprintln(in, format(t))
	}
	fail(in.Close())
	fail(ignoreExitInterrupt(fzf.Wait()))
}
