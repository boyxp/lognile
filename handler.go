package lognile

import "os"
import "sync"

type Handler struct {
	pointer *os.File
	mu sync.Mutex
}

func (H *Handler) Lock() bool {
	return H.mu.TryLock()
}

func (H *Handler) Unlock() {
	H.mu.Unlock()
}

func (H *Handler) Pointer() *os.File {
	return H.pointer
}
