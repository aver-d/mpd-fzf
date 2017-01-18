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
	"unicode/utf8"
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

type Stack struct {
	s   []string
	dir string
}

func NewStack(capacity int) *Stack {
	s := make([]string, 0, capacity)
	return &Stack{s, ""}
}

func (s *Stack) Push(dirname string) {
	s.s = append(s.s, dirname)
	s.dir = filepath.Join(s.s...)
}

func (s *Stack) Pop() {
	failOn(len(s.s) <= 0, "Invalid directory state. Corrupted database?")
	i := len(s.s) - 1
	s.s = s.s[:i]
	s.dir = filepath.Join(s.s...)
}

func (s Stack) Dir() string {
	return s.dir
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
	duration, _ := time.ParseDuration(str + "s")
	zero := time.Time{}
	format := zero.Add(duration).Format("04:05")
	if duration > time.Hour {
		format = fmt.Sprintf("%d:%s", int(duration.Hours()), format)
	}
	return "(" + format + ")"
}

func spaceBetween(left, right string, maxchars int) string {
	n_left := utf8.RuneCountInString(left)
	n_right := utf8.RuneCountInString(right)
	n := maxchars - n_left - n_right
	return left + strings.Repeat(" ", n) + right
}

func withoutExt(path string) string {
	basename := filepath.Base(path)
	return strings.TrimSuffix(basename, filepath.Ext(basename))
}

func truncate(s string, max int, suffix string) string {
	suffixLen := utf8.RuneCountInString(suffix)
	max -= suffixLen
	if max < 0 {
		panic("suffix length greater than max chars")
	}
	trunc := false
	count := 0
	out := make([]rune, 0, max)
	for _, r := range s {
		if count >= max {
			trunc = true
			break
		}
		out = append(out, r)
		count += 1
	}
	result := string(out)
	if trunc {
		result += suffix
	}
	return result
}

func trackFormatter() func(*Track) string {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	fail(err)
	var height, width int
	_, err = fmt.Sscanf(string(out), "%d %d\n", &height, &width)
	fail(err)
	contentLen := width - 5 // remove 5 for fzf display
	return func(t *Track) string {
		str := t.Artist + " - " + t.Title
		str = strings.TrimPrefix(str, " - ")
		if str == "" {
			str = withoutExt(t.Filename)
		}
		if t.Album != "" {
			str += " {" + t.Album + "}"
		}
		str = truncate(str, contentLen-len(t.Time), "..")
		str = spaceBetween(str, t.Time, contentLen)
		str = strings.Replace(str, delimiter, "", -1)
		return str + delimiter + t.Path
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

func fzfcmd() *exec.Cmd {
	bind := "--bind=ctrl-k:kill-line,enter:execute(mpd-fzf-play {})"
	fzf := exec.Command("fzf", "--no-hscroll", "--exact", bind)
	fzf.Stderr = os.Stderr
	return fzf
}

func parse(scan *bufio.Scanner) []*Track {

	tracks, track := []*Track{}, new(Track)
	dirstack := NewStack(256)

	for scan.Scan() {
		key, value := keyval(scan.Text())
		switch key {
		case "directory":
			dirstack.Push(value)
		case "end":
			dirstack.Pop()
		case "Artist", "Album", "Date", "Genre", "Time", "Title":
			track.Set(key, value)
		case "song_begin":
			track.Filename = value
			track.Path = filepath.Join(dirstack.Dir(), track.Filename)
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

func main() {
	dbFile := findDbFile()
	format := trackFormatter()

	f, err := os.Open(dbFile)
	fail(err)
	gz, err := gzip.NewReader(f)
	fail(err)

	scan := bufio.NewScanner(gz)
	tracks := groupByArtist(parse(scan))

	fail(f.Close())
	fail(gz.Close())

	fzf := fzfcmd()
	in, _ := fzf.StdinPipe()
	fail(fzf.Start())
	for _, t := range tracks {
		fmt.Fprintln(in, format(t))
	}
	fail(in.Close())
	fzf.Wait()
}
