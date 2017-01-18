# mpd-fzf

mpd-fzf is a [Music Player Daemon][mpd] (mpd) track selector.

mpd-fzf parses the mpd database and passes a list of tracks to the [fzf][fzf] command-line finder. This offers a fast way to explore a music collection interactively.

Tracks are formatted as "Artist - Track {Album} (MM:SS)", defaulting to the filename if there's insufficient information.

Running `mpd-fzf` will send the entire mpd database to fzf, and Enter key will play a track.

The compiled `mpd-fzf` binary operates with a shell script `mpd-fzf-play` (provided for bash and fish shells). Both `mpd-fzf` and `mpd-fzf-play` should be available through `$PATH`.

`mpd-fzf-play` is a copy of `mpd-fzf-play.bash`, but `mpd-fzf-play.fish` can be renamed to `mpd-fzf-play` and used instead (not that the choice really makes much difference).


## Installation

    $ git clone https://github.com/aver-d/mpd-fzf
    $ cd mpd-fzf
    $ go build -o mpd-fzf mpd-fzf.go
    # Assuming ~/bin in $PATH
    $ mv mpd-fzf mpd-fzf-play ~/bin

`mpd-fzf-play` calls [mpc][mpc] to play the track, so mpc is a dependency. I could change this to make a direct TCP connection to mpd through Go, but there doesn't seem much need given the ubiquity of mpc. The extra script also provides an opportunity to run some additional tasks related to specific mpd client.

To install mpc do something like…

    $ sudo apt-get install mpc

or

    $ sudo pacman -S mpc

## Run

This is all…

    $ mpd-fzf

Should run very fast.

____

License: MIT

[mpd]: https://www.musicpd.org
[mpc]: https://www.musicpd.org/clients/mpc
[fzf]: https://github.com/junegunn/fzf

