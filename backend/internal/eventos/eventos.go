// Package eventos difunde el avance de respaldos y restauraciones a los
// navegadores conectados por Server-Sent Events.
package eventos

import (
	"sync"
	"time"
)

type Evento struct {
	Tipo    string    `json:"tipo"` // respaldo | restauracion | programador
	ID      int64     `json:"id"`
	Estado  string    `json:"estado"`
	Mensaje string    `json:"mensaje"`
	Momento time.Time `json:"momento"`
}

type Hub struct {
	mu   sync.Mutex
	subs map[chan Evento]struct{}
}

func NuevoHub() *Hub { return &Hub{subs: map[chan Evento]struct{}{}} }

func (h *Hub) Suscribir() chan Evento {
	ch := make(chan Evento, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Cancelar(ch chan Evento) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// Publicar nunca bloquea: un cliente lento pierde eventos, no frena el motor.
func (h *Hub) Publicar(e Evento) {
	e.Momento = time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
