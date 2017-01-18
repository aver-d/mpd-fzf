#! /usr/bin/env fish

set delimiter '::::'

set fzfout "$argv[1]"
set path (echo "$fzfout" | sed -r "s/^.+$delimiter//")
set position (mpc playlist -f "%position% %file%" | grep -m1 -F "$path" | cut -d' ' -f1)

if test -z "$position"
  mpc add "$path"
  and mpc play (mpc playlist | wc -l)
else
  mpc play "$position"
end
