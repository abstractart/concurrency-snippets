#! /usr/bin/env zsh

implementations=( 
   "Stdlib" 
   #"Gosched" 
   #"SpinImplementation"
)
tasks=("cpu" "io")
TIMEFMT=$'%U \n%S \n%P \n%*E '


for implementation in ${implementations[*]}; do
    for gomaxprocs in {1..5}; do
        >&2 echo "GOMAXPROCS=$gomaxprocs TESTCASE=$implementation TASK=io"
        (time GOROUTINES=100 ITERATIONS=100 GOMAXPROCS="$gomaxprocs" TESTCASE="$implementation" TASK="io" ./main)
        >&2 echo "\n"
    done
done
