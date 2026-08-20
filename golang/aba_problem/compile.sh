#!/bin/sh
# Compile and run the C ABA demonstration.
# On Apple Silicon the 16-byte CAS is native (CASP instruction).
# On x86-64 add -mcx16 (or -march=native) for CMPXCHG16B.
set -e
clang -O1 -std=c11 -Wall -Wextra -o aba aba.c -lpthread
echo "Compiled → ./aba"
./aba
