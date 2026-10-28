set -eu

answer() {
    echo $(( $1 * 2 ))
}

for n in 1 2 3; do
    answer "$n"
done
