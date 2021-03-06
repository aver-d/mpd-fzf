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

// information | duration | path
const delimiter string = "\u2002" // EN SPACE

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

func keyValue(line string) (string, string) {
	i := strings.Index(line, ":")
	if i < 0 {
		return line, ""
	}
	if i == len(line)-1 {
		return line[:i], ""
	}
	// The value is always preceded by a space, so +2
	n := 2
	// But check anyway...
	if line[i+1] != ' ' {
		n = 1
	}
	return line[:i], line[i+n:]
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

func alignLeftRight(maxlen int, left, right string) string {
	s := runewidth.Truncate(left, maxlen, "... ")
	return runewidth.FillRight(s, maxlen) + right
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
	// Then a further one for the delimiter between info and duration.
	width := termWidth() - 5 - 1
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
		info = strings.Replace(info, delimiter, " ", -1)
		duration := formatDurationString(t.Time)
		// Right align duration
		info = alignLeftRight(width-len(duration), info, delimiter+duration)
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
		key, value := keyValue(scan.Text())
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
	if strings.HasPrefix(path, "~/") {
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

func mpcSelect(path string, play bool) {
	pos, found := mpcFindOnPlaylist(path)
	if !found {
		mpcRun("add", path)
	}
	if play {
		mpcRun("play", strconv.Itoa(pos))
	}
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

func cmdSelect(fzfline string, play bool) {
	fields := strings.SplitN(fzfline, delimiter, 3)
	if len(fields) != 3 {
		failNotify("mpd-fzf: split assertion failure")
	}
	path := fields[2]
	mpcSelect(path, play)
}

func fzfcmd() *exec.Cmd {
	bindPlay := "enter:execute-silent(mpd-fzf _play {})"
	bindQueue := "alt-enter:execute-silent(mpd-fzf _queue {})"
	fzf := exec.Command("fzf",
		"--no-hscroll",
		"--nth", "1",
		"--delimiter", delimiter,
		"--bind", bindPlay+","+bindQueue,
	)
	fzf.Stderr = os.Stderr
	return fzf
}

func ignoreExitInterrupt(err error) error {
	if err != nil && strings.HasSuffix(err.Error(), "130") {
		return nil
	}
	return err
}

func cmdList() {
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

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		if len(args) == 2 {
			cmd, path := args[0], args[1]
			// undocumented subcommands
			switch cmd {
			case "_play":
				cmdSelect(path, true)
			case "_queue":
				cmdSelect(path, false)
			}
		}
		fail(errors.New("Usage: mpd-fzf (no arguments)"))
	}
	cmdList()
}
