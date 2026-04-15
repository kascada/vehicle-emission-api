#!/bin/bash
URL="${1:-https://vehicle.akte.de/vehicle/47085?email=user@gmail.com}"
N="${2:-50}"

echo "Benchmarking $N requests to $URL"
echo ""

# Warmup (Cache füllen)
curl -s -o /dev/null "$URL"

TOTAL=0
MIN=999
MAX=0

for i in $(seq 1 "$N"); do
    T=$(curl -s -o /dev/null -w "%{time_total}" "$URL")
    MS=$(echo "$T * 1000" | bc)
    TOTAL=$(echo "$TOTAL + $MS" | bc)

    if (( $(echo "$MS < $MIN" | bc -l) )); then MIN=$MS; fi
    if (( $(echo "$MS > $MAX" | bc -l) )); then MAX=$MS; fi

    printf "\r  %d/%d" "$i" "$N"
done

AVG=$(echo "scale=1; $TOTAL / $N" | bc)
printf "\r"
echo "Results ($N requests):"
echo "  Avg: ${AVG}ms"
echo "  Min: ${MIN}ms"
echo "  Max: ${MAX}ms"
