#!/bin/sh
set -eu

answer() {
    echo $(( $1 * 2 ))
}

answer 21
