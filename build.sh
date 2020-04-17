#!/usr/bin/env bash
cd "$(dirname "${0}")" || exit
host_sys=$(uname)
case $host_sys in
Darwin*)
  if [ -d /usr/local/Cellar/ncurses ]; then
    ncurses_lib=(/usr/local/Cellar/ncurses/*/lib)
    export LIBRARY_PATH=${ncurses_lib[0]}
    export PKG_CONFIG_PATH=${ncurses_lib[0]}/pkgconfig
  fi
  go build
  ;;
Linux*)
  go build -ldflags '-w -extldflags "-static"'
  ;;
*)
  go build
  ;;
esac
