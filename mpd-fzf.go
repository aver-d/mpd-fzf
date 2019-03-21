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
	"strconv"
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
		t.Time = value
	case "Title":
		t.Title = value
	}
}

func formatDurationString(str string) string {
	duration, err := time.ParseDuration(str + "s")
	if err != nil {
		return "(-:--)"
	}
	t := time.Time{}.Add(duration)
	if duration < time.Hour {
		return t.Format("(4:05)")
	}
	return fmt.Sprintf("(%d:%s)", int(duration.Hours()), t.Format("04:05"))
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
		duration := formatDurationString(t.Time)
		// Right align duration
		info = alignLeftRight(width, info, duration)
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
	var file *os.File
	var confpath string
	for _, path := range paths {
		file, err = os.Open(path)
		if err == nil {
			confpath = path
			break
		}
	}
	failOn(file == nil, "No config file found")

	expDb := regexp.MustCompile(`^\s*db_file\s*"([^"]+)"`)
	scan := bufio.NewScanner(file)
	var dbFile string
	for scan.Scan() {
		m := expDb.FindStringSubmatch(scan.Text())
		if m != nil {
			dbFile = expandUser(m[1], home)
			break
		}
	}
	fail(scan.Err())
	fail(file.Close())
	failOn(dbFile == "", fmt.Sprintf("Could not find 'db_file' in configuration file '%s'", confpath))
	return dbFile
}

func failNotify(message string) {
	err := exec.Command("tmux", "display", message).Run()
	if err != nil {
		exec.Command("notify-send", message).Run()
	}
	fail(errors.New(message))
}

func mpcRun(args ...string) string {
	out, err := exec.Command("mpc", args...).CombinedOutput()
	if err != nil {
		failNotify(string(out))
	}
	return string(out)
}

func mpcPlay(path string) {
	pos, found := mpcFindOnPlaylist(path)
	if !found {
		mpcRun("add", path)
	}
	mpcRun("play", strconv.Itoa(pos))
}

func mpcFindOnPlaylist(path string) (int, bool) {
	playlist := mpcRun("playlist", "-f", "%file%")
	lines := strings.Split(playlist, "\n")
	for i, line := range lines {
		if line == path {
			return i + 1, true
		}
	}
	return len(lines), false
}

func cmdPlay(fzfline string) {
	fields := strings.SplitN(fzfline, delimiter, 2)
	if len(fields) != 2 {
		failNotify("mpd-fzf: assert split failure")
	}
	path := fields[1]
	mpcPlay(path)
}

func fzfcmd() *exec.Cmd {
	bind := "--bind=enter:execute-silent(mpd-fzf _play {})"
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
	if len(os.Args) > 2 && os.Args[1] == "_play" {
		cmdPlay(os.Args[2])
		return
	}
	dbFile := findDbFile()
	format := trackFormatter()

	file, err := os.Open(dbFile)
	fail(err)
	gz, err := gzip.NewReader(file)
	fail(err)

	scan := bufio.NewScanner(gz)
	tracks := groupByArtist(parse(scan))

	fail(gz.Close())
	fail(file.Close())

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
