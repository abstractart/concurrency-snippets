package cas

import (
	"fmt"
	"sync/atomic"
)

type Data struct {
	Value string
}

func Run() {
	var atomicPtr atomic.Pointer[Data]

	original := &Data{Value: "Original Data"}
	updated := &Data{Value: "Updated Data"}
	stranger := &Data{Value: "Unrelated Data"}

	atomicPtr.Store(original)

	swapped := atomicPtr.CompareAndSwap(stranger, updated)
	fmt.Printf("CAS 1 Success: %v | Current Value: %s\n", swapped, atomicPtr.Load().Value)

	swapped = atomicPtr.CompareAndSwap(original, updated)
	fmt.Printf("CAS 2 Success: %v | Current Value: %s\n", swapped, atomicPtr.Load().Value)

	updated.Value = "42"
	swapped = atomicPtr.CompareAndSwap(updated, stranger)
	fmt.Printf("CAS 3 Success: %v | Current Value: %s\n", swapped, atomicPtr.Load().Value)
}
