package videoserver

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// https://github.com/trailofbits/go-mutexasserts/blob/master/mutex.go#L15
const mutexLocked = 1

func RWMutexLocked(rw *sync.RWMutex) bool {
	// RWMutex has a "w" sync.Mutex field for write lock
	state := reflect.ValueOf(rw).Elem().FieldByName("w").FieldByName("state")
	return state.Int()&mutexLocked == mutexLocked
}

func MutexLocked(m *sync.Mutex) bool {
	state := reflect.ValueOf(m).Elem().FieldByName("state")
	return state.Int()&mutexLocked == mutexLocked
}

func RWMutexRLocked(rw *sync.RWMutex) bool {
	return readerCount(rw) > 0
}

// Starting in go1.20, readerCount is an atomic int32 value.
// See: https://go-review.googlesource.com/c/go/+/429767
func readerCount(rw *sync.RWMutex) int64 {
	// Look up the address of the readerCount field and use it to create a pointer to an atomic.Int32,
	// then load the value to return.
	rc := (*atomic.Int32)(reflect.ValueOf(rw).Elem().FieldByName("readerCount").Addr().UnsafePointer())
	return int64(rc.Load())
}

// Prior to go1.20, readerCount was an int value.
// func readerCount(rw *sync.RWMutex) int64 {
// return reflect.ValueOf(rw).Elem().FieldByName("readerCount").Int()
// }
