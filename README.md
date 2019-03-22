# mpd-fzf

mpd-fzf is a [Music Player Daemon][mpd] (mpd) track selector.

mpd-fzf parses the mpd database and passes a list of tracks to the [fzf][fzf] command-line finder. This offers a fast way to explore a music collection interactively.

Tracks are formatted as "Artist - Track {Album} (MM:SS)", defaulting to the filename if there's insufficient information.

mpd-fzf may be a useful companion to an mpd client such as [ncmpc][ncmpc] or [ncmpcpp][ncmpcpp]. Tracks selected in fzf will pop up on the main client's playlist in another terminal.


## Usage

    $ mpd-fzf

This will send the entire mpd database to fzf. The following keys can be used:

* Enter: play track
* Alt-Enter: add track to playlist if it's not already listed

All other fzf keybindings are as normal. These include Escape or Ctrl-Q to exit.

I'm unlikely to add any further features since there are already many other mpd clients which work well. Note also that mpd-fzf is not intended to be used in scripts. If, for example, you'd like to use fzf to select and pass on tracks, you can use mpc (installation mentioned below).

    $ mpc listall -f 'your_format_string' | fzf -m | your-script

In fact, on that afternoon when I first wrote this program, if I'd noticed that mpc can do this, then I probably wouldn't have bothered writing the section of code which parses the mpd database (though the parsing is straightforward to do and provided some insight into how mpd works).


## Installation

    $ go get -u github.com/aver-d/mpd-fzf

Using `go get` you may also need to set the [GOPATH][gopath] environment variable.

Alternatively, assuming `~/bin` in `$PATH`, you could also do

    $ go build -o ~/bin/mpd-fzf mpd-fzf.go

mpd-fzf calls [mpc][mpc] to play the track, so mpc is a dependency. I could change this to make a direct TCP connection to mpd through Go, but there doesn't seem much need given the ubiquity of mpc.

To install mpc do something like…

    $ sudo apt-get install mpc

or

    $ sudo pacman -S mpc

____

License: MIT

[mpd]: https://www.musicpd.org
[mpc]: https://www.musicpd.org/clients/mpc
[ncmpc]: https://www.musicpd.org/clients/ncmpc
[fzf]: https://github.com/junegunn/fzf
[gopath]: https://github.com/golang/go/wiki/GOPATH
[ncmpcpp]: https://github.com/arybczak/ncmpcpp
