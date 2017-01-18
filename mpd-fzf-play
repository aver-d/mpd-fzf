#! /usr/bin/env bash

delimiter='::::'

fzfout="$1"
path=$(echo "$fzfout" | sed -r "s/^.+$delimiter//")
position=$(mpc playlist -f "%position% %file%" | grep -m1 -F "$path" | cut -d' ' -f1)

if [ -z "$position" ]; then
  mpc add "$path"
  mpc play $(mpc playlist | wc -l)
else
  mpc play "$position"
fi
