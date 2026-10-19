package hub

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// Bus is a stereo mix-bus entry for the UI (odd bus number).
type Bus struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

// Channel is a submix send strip.
type Channel struct {
	Number    int     `json:"number"`
	Name      string  `json:"name"`
	Level     float32 `json:"level"`
	Pan       float32 `json:"pan"`
	On        bool    `json:"on"`
	Icon      int     `json:"icon"`
	IconEmoji string  `json:"iconEmoji"`
	Color     int     `json:"color"`
	ColorCSS  string  `json:"colorCss"`
}

// Status is the per-client view pushed over WebSocket.
type Status struct {
	SessionID       string    `json:"sessionId,omitempty"`
	Phase           string    `json:"phase"` // idle | searching | connected | mixing | error
	Message         string    `json:"message,omitempty"`
	IP              string    `json:"ip,omitempty"`
	Port            int       `json:"port,omitempty"`
	Name            string    `json:"name,omitempty"`
	Model           string    `json:"model,omitempty"`
	Firmware        string    `json:"firmware,omitempty"`
	Buses           []Bus     `json:"buses,omitempty"`
	SelectedBus     int       `json:"selectedBus,omitempty"`
	SelectedBusName string    `json:"selectedBusName,omitempty"`
	Channels        []Channel `json:"channels,omitempty"`
}

type envelope struct {
	Type string `json:"type"`
	Data Status `json:"data"`
}

type metersEnvelope struct {
	Type string        `json:"type"`
	Data metersPayload `json:"data"`
}

type metersPayload struct {
	Levels []float32 `json:"levels"`
}

// global holds mixer connection state shared by all sessions.
type global struct {
	Phase    string // idle | searching | ready | error
	Message  string
	IP       string
	Port     int
	Name     string
	Model    string
	Firmware string
	Buses    []Bus
}

type session struct {
	id              string
	phase           string // connected | mixing (view); empty uses global
	message         string
	selectedBus     int
	selectedBusName string
	preferredBus    int // last chosen mix; survives clear / reconnect
	channels        []Channel
	conn            *websocket.Conn
}

// Hub keeps shared mixer state and independent per-browser sessions.
type Hub struct {
	mu       sync.RWMutex
	writeMu  sync.Mutex
	global   global
	sessions map[string]*session
}

// New creates a hub in idle state.
func New() *Hub {
	return &Hub{
		global: global{
			Phase:   "idle",
			Message: "Bereit",
		},
		sessions: make(map[string]*session),
	}
}

// GlobalPhase returns the shared mixer link phase.
func (h *Hub) GlobalPhase() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.global.Phase
}

// MixerAddr returns connected mixer IP/port when known.
func (h *Hub) MixerAddr() (ip string, port int, ok bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.global.IP == "" || h.global.Port == 0 {
		return "", 0, false
	}
	return h.global.IP, h.global.Port, true
}

// EnsureSession returns an existing session or creates one.
// If the client provides an ID unknown to the server (e.g. after restart), it is reused.
func (h *Hub) EnsureSession(id string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if id != "" {
		if _, ok := h.sessions[id]; ok {
			return id
		}
		h.sessions[id] = &session{id: id}
		return id
	}
	id = newSessionID()
	h.sessions[id] = &session{id: id}
	return id
}

// BindConn attaches a websocket to a session and sends the current view.
func (h *Hub) BindConn(id string, conn *websocket.Conn) {
	h.mu.Lock()
	sess := h.sessions[id]
	if sess == nil {
		sess = &session{id: id}
		h.sessions[id] = sess
	}
	if sess.conn != nil && sess.conn != conn {
		_ = sess.conn.Close()
	}
	sess.conn = conn
	view := h.viewLocked(sess)
	h.mu.Unlock()
	_ = h.writeStatus(conn, view)
}

// UnbindConn detaches a websocket; session state is kept for reconnect.
func (h *Hub) UnbindConn(id string, conn *websocket.Conn) {
	h.mu.Lock()
	if sess, ok := h.sessions[id]; ok && sess.conn == conn {
		sess.conn = nil
	}
	h.mu.Unlock()
	_ = conn.Close()
}

// View returns the merged status for a session.
func (h *Hub) View(id string) Status {
	h.mu.RLock()
	defer h.mu.RUnlock()
	sess := h.sessions[id]
	if sess == nil {
		return h.viewLocked(&session{id: id})
	}
	return h.viewLocked(sess)
}

// SetGlobal updates shared mixer state and notifies all sessions.
func (h *Hub) SetGlobal(phase, message, ip string, port int, name, model, firmware string, buses []Bus) {
	h.mu.Lock()
	h.global = global{
		Phase:    phase,
		Message:  message,
		IP:       ip,
		Port:     port,
		Name:     name,
		Model:    model,
		Firmware: firmware,
		Buses:    append([]Bus(nil), buses...),
	}
	// Reset active mix views when connection drops; keep preferredBus.
	if phase == "searching" || phase == "error" || phase == "idle" {
		for _, sess := range h.sessions {
			sess.phase = ""
			sess.message = ""
			sess.selectedBus = 0
			sess.selectedBusName = ""
			sess.channels = nil
		}
	}
	views := h.allViewsLocked()
	h.mu.Unlock()
	for _, v := range views {
		h.push(v.id, v.status)
	}
}

// SetBuses updates the shared stereo-bus list without resetting sessions.
func (h *Hub) SetBuses(buses []Bus) {
	h.mu.Lock()
	h.global.Buses = append([]Bus(nil), buses...)
	for _, sess := range h.sessions {
		if sess.selectedBus == 0 {
			continue
		}
		for _, b := range buses {
			if b.Number == sess.selectedBus {
				sess.selectedBusName = b.Name
				break
			}
		}
	}
	views := h.allViewsLocked()
	h.mu.Unlock()
	for _, v := range views {
		h.push(v.id, v.status)
	}
}

// SetSessionView updates one session's bus/mixer view.
func (h *Hub) SetSessionView(id, phase, message string, bus int, busName string, channels []Channel) {
	h.mu.Lock()
	sess := h.sessions[id]
	if sess == nil {
		sess = &session{id: id}
		h.sessions[id] = sess
	}
	sess.phase = phase
	sess.message = message
	sess.selectedBus = bus
	sess.selectedBusName = busName
	if phase == "mixing" && bus > 0 {
		sess.preferredBus = bus
	}
	if channels != nil {
		sess.channels = append([]Channel(nil), channels...)
	} else {
		sess.channels = nil
	}
	view := h.viewLocked(sess)
	h.mu.Unlock()
	h.push(id, view)
}

// ClearSessionBus returns a session to bus selection (keeps preferredBus).
func (h *Hub) ClearSessionBus(id string) bool {
	h.mu.Lock()
	sess := h.sessions[id]
	if sess == nil {
		h.mu.Unlock()
		return false
	}
	if h.global.Phase != "ready" {
		h.mu.Unlock()
		return false
	}
	if sess.selectedBus > 0 {
		sess.preferredBus = sess.selectedBus
	}
	sess.phase = "connected"
	sess.message = "Loading buses…"
	sess.selectedBus = 0
	sess.selectedBusName = ""
	sess.channels = nil
	view := h.viewLocked(sess)
	h.mu.Unlock()
	h.push(id, view)
	return true
}

// PreferredBus returns the last chosen mix for a session, if any.
func (h *Hub) PreferredBus(id string) (int, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	sess := h.sessions[id]
	if sess == nil || sess.preferredBus == 0 {
		return 0, false
	}
	for _, b := range h.global.Buses {
		if b.Number == sess.preferredBus {
			return sess.preferredBus, true
		}
	}
	return 0, false
}

// PreferredBusRef is a session that should be restored to a mix.
type PreferredBusRef struct {
	ID  string
	Bus int
}

// ListPreferredBuses returns sessions with a saved mix (for restore after connect).
func (h *Hub) ListPreferredBuses() []PreferredBusRef {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []PreferredBusRef
	if h.global.Phase != "ready" {
		return out
	}
	for _, sess := range h.sessions {
		if sess.preferredBus == 0 || sess.phase == "mixing" {
			continue
		}
		for _, b := range h.global.Buses {
			if b.Number == sess.preferredBus {
				out = append(out, PreferredBusRef{ID: sess.id, Bus: sess.preferredBus})
				break
			}
		}
	}
	return out
}

// SetPreferredBus stores the last mix without changing the active view.
func (h *Hub) SetPreferredBus(id string, bus int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sess := h.sessions[id]
	if sess == nil {
		sess = &session{id: id}
		h.sessions[id] = sess
	}
	if bus > 0 {
		sess.preferredBus = bus
	}
}

// SessionBus returns the selected bus for a mixing session.
func (h *Hub) SessionBus(id string) (bus int, ok bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	sess := h.sessions[id]
	if sess == nil || sess.phase != "mixing" || sess.selectedBus == 0 {
		return 0, false
	}
	return sess.selectedBus, true
}

// PatchSessionChannel updates one channel field in a session and notifies it.
func (h *Hub) PatchSessionChannel(id string, channel int, fn func(*Channel) bool) {
	h.mu.Lock()
	sess := h.sessions[id]
	if sess == nil || sess.phase != "mixing" {
		h.mu.Unlock()
		return
	}
	changed := false
	for i := range sess.channels {
		if sess.channels[i].Number == channel {
			changed = fn(&sess.channels[i])
			break
		}
	}
	view := h.viewLocked(sess)
	h.mu.Unlock()
	if changed {
		h.push(id, view)
	}
}

// ApplyOSCUpdate applies a live update to all sessions watching the bus.
func (h *Hub) ApplyOSCUpdate(bus, channel int, kind string, apply func(*Channel) bool) {
	type push struct {
		id     string
		status Status
	}
	var out []push
	h.mu.Lock()
	for _, sess := range h.sessions {
		if sess.phase != "mixing" || sess.selectedBus == 0 {
			continue
		}
		if bus != 0 && bus != sess.selectedBus && bus != sess.selectedBus+1 {
			continue
		}
		changed := false
		for i := range sess.channels {
			if sess.channels[i].Number != channel {
				continue
			}
			changed = apply(&sess.channels[i])
			break
		}
		if changed {
			out = append(out, push{id: sess.id, status: h.viewLocked(sess)})
		}
	}
	h.mu.Unlock()
	for _, p := range out {
		h.push(p.id, p.status)
	}
}

// BusName looks up a stereo bus name from the global list.
func (h *Hub) BusName(bus int) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, b := range h.global.Buses {
		if b.Number == bus {
			return b.Name, true
		}
	}
	return "", false
}

type viewPush struct {
	id     string
	status Status
}

func (h *Hub) allViewsLocked() []viewPush {
	out := make([]viewPush, 0, len(h.sessions))
	for _, sess := range h.sessions {
		out = append(out, viewPush{id: sess.id, status: h.viewLocked(sess)})
	}
	return out
}

func (h *Hub) viewLocked(sess *session) Status {
	g := h.global
	st := Status{
		SessionID: sess.id,
		IP:        g.IP,
		Port:      g.Port,
		Name:      g.Name,
		Model:     g.Model,
		Firmware:  g.Firmware,
		Buses:     append([]Bus(nil), g.Buses...),
	}

	switch g.Phase {
	case "idle":
		st.Phase = "idle"
		st.Message = g.Message
	case "searching":
		st.Phase = "searching"
		st.Message = g.Message
	case "error":
		st.Phase = "error"
		st.Message = g.Message
	case "ready":
		if sess.phase == "mixing" {
			st.Phase = "mixing"
			st.Message = sess.message
			if st.Message == "" {
				st.Message = sess.selectedBusName
			}
			st.SelectedBus = sess.selectedBus
			st.SelectedBusName = sess.selectedBusName
			st.Channels = append([]Channel(nil), sess.channels...)
		} else {
			st.Phase = "connected"
			st.Message = sess.message
			if st.Message == "" {
				st.Message = "Choose a bus"
			}
		}
	default:
		st.Phase = "idle"
		st.Message = g.Message
	}
	return st
}

func (h *Hub) push(id string, st Status) {
	h.mu.RLock()
	sess := h.sessions[id]
	var conn *websocket.Conn
	if sess != nil {
		conn = sess.conn
	}
	h.mu.RUnlock()
	if conn == nil {
		return
	}
	if err := h.writeStatus(conn, st); err != nil {
		h.UnbindConn(id, conn)
	}
}

func (h *Hub) writeStatus(conn *websocket.Conn, s Status) error {
	payload, err := json.Marshal(envelope{Type: "status", Data: s})
	if err != nil {
		return err
	}
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, payload)
}

// BroadcastMeters pushes channel input meters to all connected browsers (lightweight).
func (h *Hub) BroadcastMeters(levels []float32) {
	if len(levels) == 0 {
		return
	}
	payload, err := json.Marshal(metersEnvelope{
		Type: "meters",
		Data: metersPayload{Levels: levels},
	})
	if err != nil {
		return
	}
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.sessions))
	for _, sess := range h.sessions {
		if sess.conn != nil {
			conns = append(conns, sess.conn)
		}
	}
	h.mu.RUnlock()
	if len(conns) == 0 {
		return
	}
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	for _, conn := range conns {
		_ = conn.WriteMessage(websocket.TextMessage, payload)
	}
}

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte("fallback")) + hex.EncodeToString(b[:8])
	}
	return hex.EncodeToString(b[:])
}
