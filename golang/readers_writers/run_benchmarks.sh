#!/usr/bin/env bash
set -euo pipefail

# Move to the script directory (so it runs for the package in this folder)
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

# Find benchmark function names in .go files (e.g. func BenchmarkMRead(b *testing.B) { ... })
benchmarks=$(grep -h -E '^func +Benchmark[[:alnum:]_]*\(' *.go 2>/dev/null || true)

if [ -z "$benchmarks" ]; then
  echo "No benchmarks found in $DIR"
  exit 1
fi

# Extract the function names and de-duplicate
benchmarks=$(printf "%s\n" "$benchmarks" | sed -E 's/func +([^(]+)\(.*/\1/' | sort -u)

echo "Found benchmarks:"
printf "  %s\n" $benchmarks

for b in $benchmarks; do
  echo "Running benchmark: $b"
  go test -bench="$b" -benchmem \
    -mutexprofile="profiles/${b}_mutex_profile.out" \
    -cpuprofile="profiles/${b}_cpu_profile.out" \
    -memprofile="profiles/${b}_memory_profile.out"
done

echo "All benchmarks finished. Profile files are in: $DIR"
