package main

import (
	"fmt"
	"sync/atomic"
)

type Data struct {
	Value string
}

func main() {
	// Initialize an atomic pointer managing a *Data type
	var atomicPtr atomic.Pointer[Data]

	original := &Data{Value: "Original Data"}
	updated := &Data{Value: "Updated Data"}
	stranger := &Data{Value: "Unrelated Data"}

	// 1. Initial store
	atomicPtr.Store(original)

	// 2. This CAS will FAIL because 'stranger' does not match 'original'
	swapped := atomicPtr.CompareAndSwap(stranger, updated)
	fmt.Printf("CAS 1 Success: %v | Current Value: %s\n", swapped, atomicPtr.Load().Value)
	// Output: CAS 1 Success: false | Current Value: Original Data

	// 3. This CAS will SUCCEED because the expected value matches 'original'
	swapped = atomicPtr.CompareAndSwap(original, updated)
	fmt.Printf("CAS 2 Success: %v | Current Value: %s\n", swapped, atomicPtr.Load().Value)
	// Output: CAS 2 Success: true | Current Value: Updated Data

	updated.Value = "42"

	swapped = atomicPtr.CompareAndSwap(updated, stranger)
	fmt.Printf("CAS 3 Success: %v | Current Value: %s\n", swapped, atomicPtr.Load().Value)

}
