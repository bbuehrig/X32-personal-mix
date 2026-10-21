package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"X32-personal-mix/internal/hub"
	"X32-personal-mix/internal/osc"
	"X32-personal-mix/internal/x32"

	"github.com/gorilla/websocket"
)

const sessionHeader = "X-Session-ID"

// MixerAddr is a fixed mixer target that skips LAN discovery.
type MixerAddr struct {
	IP   string
	Port int
}

// Server serves the web UI and coordinates X32 discovery.
type Server struct {
	Hub      *hub.Hub
	OSC      *osc.Client
	WebFS    fs.FS
	Fixed    *MixerAddr
	mu       sync.Mutex
	busy     bool
	upgrader websocket.Upgrader

	remoteMu   sync.Mutex
	remoteStop chan struct{}
}

// New creates an API server. If fixed is non-nil, discovery is skipped.
func New(h *hub.Hub, oscClient *osc.Client, webFS fs.FS, fixed *MixerAddr) *Server {
	s := &Server{
		Hub:   h,
		OSC:   oscClient,
		WebFS: webFS,
		Fixed: fixed,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	oscClient.OnUpdate(s.handleOSCUpdate)
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/discover", s.handleDiscover)
	mux.HandleFunc("POST /api/select-bus", s.handleSelectBus)
	mux.HandleFunc("POST /api/clear-bus", s.handleClearBus)
	mux.HandleFunc("POST /api/level", s.handleLevel)
	mux.HandleFunc("POST /api/pan", s.handlePan)
	mux.HandleFunc("POST /api/mute", s.handleMute)
	mux.HandleFunc("POST /api/bus-level", s.handleBusLevel)
	mux.HandleFunc("POST /api/bus-pan", s.handleBusPan)
	mux.HandleFunc("POST /api/bus-mute", s.handleBusMute)
	mux.HandleFunc("GET /ws", s.handleWS)
	fileServer := http.FileServer(http.FS(s.WebFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// PWA / apple-touch icons: avoid long browser caches so home-screen picks stay current.
		path := r.URL.Path
		if path == "/apple-touch-icon.png" || path == "/apple-touch-icon-v33.png" ||
			path == "/icon-192.png" || path == "/icon-192-v33.png" ||
			path == "/icon-512.png" || path == "/icon-512-v33.png" ||
			path == "/favicon.svg" || path == "/favicon-16.png" || path == "/favicon-32.png" ||
			path == "/manifest.json" || path == "/sw.js" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
	return mux
}

func (s *Server) sessionID(r *http.Request) string {
	id := r.Header.Get(sessionHeader)
	if id == "" {
		id = r.URL.Query().Get("sid")
	}
	return s.Hub.EnsureSession(id)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(sessionHeader, id)
	_ = json.NewEncoder(w).Encode(s.Hub.View(id))
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	go s.Connect(false)
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

type selectBusRequest struct {
	Bus int `json:"bus"`
}

func (s *Server) handleSelectBus(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	var req selectBusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	go s.SelectBus(id, req.Bus)
	w.Header().Set(sessionHeader, id)
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Server) handleClearBus(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	if !s.Hub.ClearSessionBus(id) {
		http.Error(w, `{"error":"not connected"}`, http.StatusConflict)
		return
	}
	go s.refreshBusesForPick(id)
	w.Header().Set(sessionHeader, id)
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

type levelRequest struct {
	Channel int     `json:"channel"`
	Level   float32 `json:"level"`
}

func (s *Server) handleLevel(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	var req levelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	bus, ok := s.Hub.SessionBus(id)
	if !ok {
		http.Error(w, `{"error":"no submix selected"}`, http.StatusConflict)
		return
	}
	ip, port, ok := s.Hub.MixerAddr()
	if !ok {
		http.Error(w, `{"error":"mixer offline"}`, http.StatusConflict)
		return
	}
	if req.Channel < 1 || req.Channel > x32.ChannelCount {
		http.Error(w, `{"error":"invalid channel"}`, http.StatusBadRequest)
		return
	}
	if err := x32.SetSendLevel(s.OSC, ip, port, req.Channel, bus, req.Level); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	level := clamp01(req.Level)
	s.Hub.ApplyOSCUpdate(bus, req.Channel, "level", func(ch *hub.Channel) bool {
		if ch.Level == level {
			return false
		}
		ch.Level = level
		return true
	})
	w.Header().Set(sessionHeader, id)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

type panRequest struct {
	Channel int     `json:"channel"`
	Pan     float32 `json:"pan"`
}

func (s *Server) handlePan(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	var req panRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	bus, ok := s.Hub.SessionBus(id)
	if !ok {
		http.Error(w, `{"error":"no submix selected"}`, http.StatusConflict)
		return
	}
	ip, port, ok := s.Hub.MixerAddr()
	if !ok {
		http.Error(w, `{"error":"mixer offline"}`, http.StatusConflict)
		return
	}
	if req.Channel < 1 || req.Channel > x32.ChannelCount {
		http.Error(w, `{"error":"invalid channel"}`, http.StatusBadRequest)
		return
	}
	if err := x32.SetSendPan(s.OSC, ip, port, req.Channel, bus, req.Pan); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	pan := clamp01(req.Pan)
	s.Hub.ApplyOSCUpdate(bus, req.Channel, "pan", func(ch *hub.Channel) bool {
		if ch.Pan == pan {
			return false
		}
		ch.Pan = pan
		return true
	})
	w.Header().Set(sessionHeader, id)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

type muteRequest struct {
	Channel int  `json:"channel"`
	On      bool `json:"on"`
}

func (s *Server) handleMute(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	var req muteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	bus, ok := s.Hub.SessionBus(id)
	if !ok {
		http.Error(w, `{"error":"no submix selected"}`, http.StatusConflict)
		return
	}
	ip, port, ok := s.Hub.MixerAddr()
	if !ok {
		http.Error(w, `{"error":"mixer offline"}`, http.StatusConflict)
		return
	}
	if req.Channel < 1 || req.Channel > x32.ChannelCount {
		http.Error(w, `{"error":"invalid channel"}`, http.StatusBadRequest)
		return
	}
	if err := x32.SetSendOn(s.OSC, ip, port, req.Channel, bus, req.On); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	s.Hub.ApplyOSCUpdate(bus, req.Channel, "on", func(ch *hub.Channel) bool {
		if ch.On == req.On {
			return false
		}
		ch.On = req.On
		return true
	})
	w.Header().Set(sessionHeader, id)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

type busLevelRequest struct {
	Level float32 `json:"level"`
}

func (s *Server) handleBusLevel(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	var req busLevelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	bus, ok := s.Hub.SessionBus(id)
	if !ok {
		http.Error(w, `{"error":"no submix selected"}`, http.StatusConflict)
		return
	}
	ip, port, ok := s.Hub.MixerAddr()
	if !ok {
		http.Error(w, `{"error":"mixer offline"}`, http.StatusConflict)
		return
	}
	if err := x32.SetBusFader(s.OSC, ip, port, bus, req.Level); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	level := clamp01(req.Level)
	s.Hub.ApplyBusMasterUpdate(bus, func(m *hub.BusMaster) bool {
		if m.Level == level {
			return false
		}
		m.Level = level
		return true
	})
	w.Header().Set(sessionHeader, id)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

type busPanRequest struct {
	Pan float32 `json:"pan"`
}

func (s *Server) handleBusPan(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	var req busPanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	bus, ok := s.Hub.SessionBus(id)
	if !ok {
		http.Error(w, `{"error":"no submix selected"}`, http.StatusConflict)
		return
	}
	ip, port, ok := s.Hub.MixerAddr()
	if !ok {
		http.Error(w, `{"error":"mixer offline"}`, http.StatusConflict)
		return
	}
	if err := x32.SetBusPan(s.OSC, ip, port, bus, req.Pan); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	pan := clamp01(req.Pan)
	s.Hub.ApplyBusMasterUpdate(bus, func(m *hub.BusMaster) bool {
		if m.Pan == pan {
			return false
		}
		m.Pan = pan
		return true
	})
	w.Header().Set(sessionHeader, id)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

type busMuteRequest struct {
	On bool `json:"on"`
}

func (s *Server) handleBusMute(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	var req busMuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	bus, ok := s.Hub.SessionBus(id)
	if !ok {
		http.Error(w, `{"error":"no submix selected"}`, http.StatusConflict)
		return
	}
	ip, port, ok := s.Hub.MixerAddr()
	if !ok {
		http.Error(w, `{"error":"mixer offline"}`, http.StatusConflict)
		return
	}
	if err := x32.SetBusOn(s.OSC, ip, port, bus, req.On); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	s.Hub.ApplyBusMasterUpdate(bus, func(m *hub.BusMaster) bool {
		if m.On == req.On {
			return false
		}
		m.On = req.On
		return true
	})
	w.Header().Set(sessionHeader, id)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}
	id := s.sessionID(r)
	s.Hub.BindConn(id, conn)

	switch s.Hub.GlobalPhase() {
	case "idle":
		go s.Connect(true)
	case "ready":
		view := s.Hub.View(id)
		if view.Phase != "mixing" {
			if bus, ok := s.Hub.PreferredBus(id); ok {
				go s.SelectBus(id, bus)
			} else {
				go s.refreshBusesForPick(id)
			}
		}
	}

	defer s.Hub.UnbindConn(id, conn)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (s *Server) handleOSCUpdate(msg osc.Message) {
	if levels, ok := x32.ParseMeters0(msg); ok {
		s.Hub.BroadcastMeters(levels)
		return
	}
	upd, ok := x32.ParseSubmixUpdate(msg)
	if !ok {
		return
	}
	if upd.Channel == 0 {
		switch upd.Kind {
		case "name", "icon", "color":
			s.Hub.PatchBusList(upd.Bus, func(b *hub.Bus) bool {
				switch upd.Kind {
				case "name":
					if b.Name == upd.Name {
						return false
					}
					b.Name = upd.Name
				case "icon":
					if b.Icon == upd.Icon {
						return false
					}
					b.Icon = upd.Icon
					b.IconSrc = x32.IconSrc(upd.Icon)
				case "color":
					if b.Color == upd.Color {
						return false
					}
					b.Color = upd.Color
					b.ColorCSS = x32.ColorCSS(upd.Color)
				}
				return true
			})
		}
		s.Hub.ApplyBusMasterUpdate(upd.Bus, func(m *hub.BusMaster) bool {
			switch upd.Kind {
			case "level":
				if m.Level == upd.Level {
					return false
				}
				m.Level = upd.Level
			case "pan":
				if m.Pan == upd.Pan {
					return false
				}
				m.Pan = upd.Pan
			case "on":
				if m.On == upd.On {
					return false
				}
				m.On = upd.On
			case "name":
				if m.Name == upd.Name {
					return false
				}
				m.Name = upd.Name
			case "icon":
				if m.Icon == upd.Icon {
					return false
				}
				m.Icon = upd.Icon
				m.IconSrc = x32.IconSrc(upd.Icon)
			case "color":
				if m.Color == upd.Color {
					return false
				}
				m.Color = upd.Color
				m.ColorCSS = x32.ColorCSS(upd.Color)
			default:
				return false
			}
			return true
		})
		return
	}
	s.Hub.ApplyOSCUpdate(upd.Bus, upd.Channel, upd.Kind, func(ch *hub.Channel) bool {
		switch upd.Kind {
		case "level":
			if ch.Level == upd.Level {
				return false
			}
			ch.Level = upd.Level
		case "pan":
			if ch.Pan == upd.Pan {
				return false
			}
			ch.Pan = upd.Pan
		case "on":
			if ch.On == upd.On {
				return false
			}
			ch.On = upd.On
		case "name":
			if ch.Name == upd.Name {
				return false
			}
			ch.Name = upd.Name
		case "icon":
			if ch.Icon == upd.Icon {
				return false
			}
			ch.Icon = upd.Icon
			ch.IconSrc = x32.IconSrc(upd.Icon)
		case "color":
			if ch.Color == upd.Color {
				return false
			}
			ch.Color = upd.Color
			ch.ColorCSS = x32.ColorCSS(upd.Color)
		default:
			return false
		}
		return true
	})
}

// Connect establishes a mixer link. With Fixed set, discovery is skipped.
func (s *Server) Connect(auto bool) {
	if !s.beginWork(auto) {
		return
	}
	defer s.endWork()

	s.stopLiveOSC()

	if s.Fixed != nil {
		s.Hub.SetGlobal("searching", "Connecting…", "", 0, "", "", "", nil)
		m, err := x32.QueryXinfo(s.OSC, s.Fixed.IP, s.Fixed.Port, 1500*time.Millisecond)
		if err != nil {
			s.Hub.SetGlobal("error", "No response: "+err.Error(), "", 0, "", "", "", nil)
			return
		}
		if m.IP == "" {
			m.IP = s.Fixed.IP
		}
		m.Port = s.Fixed.Port
		s.applyMixer(m)
		return
	}

	s.Hub.SetGlobal("searching", "Searching…", "", 0, "", "", "", nil)

	mixers, err := x32.Discover(s.OSC, 1500*time.Millisecond)
	if err != nil {
		s.Hub.SetGlobal("error", "Discovery failed: "+err.Error(), "", 0, "", "", "", nil)
		return
	}
	if len(mixers) == 0 {
		s.Hub.SetGlobal("error", "No X32 found", "", 0, "", "", "", nil)
		return
	}

	s.applyMixer(mixers[0])
}

func (s *Server) applyMixer(m x32.Mixer) {
	s.Hub.SetGlobal("searching", "Loading buses…", m.IP, m.Port, m.Name, m.Model, m.Firmware, nil)

	buses, err := x32.FetchStereoBuses(s.OSC, m.IP, m.Port, 800*time.Millisecond)
	if err != nil {
		s.Hub.SetGlobal("error", "Could not load buses: "+err.Error(), m.IP, m.Port, m.Name, m.Model, m.Firmware, nil)
		return
	}

	s.startLiveOSC(m.IP, m.Port)
	s.Hub.SetGlobal("ready", "Choose a bus", m.IP, m.Port, m.Name, m.Model, m.Firmware, toHubBuses(buses))
	go s.restorePreferredBuses()
}

func (s *Server) restorePreferredBuses() {
	for _, ref := range s.Hub.ListPreferredBuses() {
		s.SelectBus(ref.ID, ref.Bus)
	}
}

// refreshBusList re-reads stereo bus names from the X32 into the hub.
func (s *Server) refreshBusList() error {
	ip, port, ok := s.Hub.MixerAddr()
	if !ok {
		return fmt.Errorf("mixer offline")
	}
	buses, err := x32.FetchStereoBuses(s.OSC, ip, port, 600*time.Millisecond)
	if err != nil {
		return err
	}
	s.Hub.SetBuses(toHubBuses(buses))
	return nil
}

// refreshBusesForPick reloads bus names when a session opens the bus picker.
func (s *Server) refreshBusesForPick(sessionID string) {
	if view := s.Hub.View(sessionID); view.Phase == "connected" {
		s.Hub.SetSessionView(sessionID, "connected", "Loading buses…", 0, "", nil, nil)
	}
	if err := s.refreshBusList(); err != nil {
		log.Printf("refresh buses: %v", err)
	}
	if view := s.Hub.View(sessionID); view.Phase == "connected" {
		s.Hub.SetSessionView(sessionID, "connected", "Choose a bus", 0, "", nil, nil)
	}
}

// SelectBus loads the channel submix for one browser session.
func (s *Server) SelectBus(sessionID string, bus int) {
	if !s.beginWork(false) {
		return
	}
	defer s.endWork()

	if s.Hub.GlobalPhase() != "ready" {
		return
	}
	busName, ok := s.Hub.BusName(bus)
	if !ok {
		return
	}
	ip, port, ok := s.Hub.MixerAddr()
	if !ok {
		return
	}

	s.Hub.SetSessionView(sessionID, "connected", "Loading submix…", bus, busName, nil, nil)

	channels, err := x32.FetchSubmix(s.OSC, ip, port, bus, 400*time.Millisecond)
	if err != nil {
		s.Hub.SetSessionView(sessionID, "connected", "Submix failed", 0, "", nil, nil)
		return
	}
	master, err := x32.FetchBusMaster(s.OSC, ip, port, bus, busName, 400*time.Millisecond)
	if err != nil {
		s.Hub.SetSessionView(sessionID, "connected", "Submix failed", 0, "", nil, nil)
		return
	}

	s.Hub.SetSessionView(sessionID, "mixing", busName, bus, busName, toHubChannels(channels), toHubBusMaster(master))
}

func toHubChannels(channels []x32.Channel) []hub.Channel {
	hubCh := make([]hub.Channel, len(channels))
	for i, c := range channels {
		hubCh[i] = hub.Channel{
			Number:   c.Number,
			Name:     c.Name,
			Level:    c.Level,
			Pan:      c.Pan,
			On:       c.On,
			Icon:     c.Icon,
			IconSrc:  c.IconSrc,
			Color:    c.Color,
			ColorCSS: c.ColorCSS,
		}
	}
	return hubCh
}

func toHubBuses(buses []x32.Bus) []hub.Bus {
	out := make([]hub.Bus, len(buses))
	for i, b := range buses {
		out[i] = hub.Bus{
			Number:   b.Number,
			Name:     b.Name,
			Icon:     b.Icon,
			IconSrc:  b.IconSrc,
			Color:    b.Color,
			ColorCSS: b.ColorCSS,
		}
	}
	return out
}

func toHubBusMaster(m *x32.BusMaster) *hub.BusMaster {
	if m == nil {
		return nil
	}
	return &hub.BusMaster{
		Number:   m.Number,
		Name:     m.Name,
		Level:    m.Level,
		Pan:      m.Pan,
		On:       m.On,
		Icon:     m.Icon,
		IconSrc:  m.IconSrc,
		Color:    m.Color,
		ColorCSS: m.ColorCSS,
	}
}

func (s *Server) startLiveOSC(host string, port int) {
	s.stopLiveOSC()
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		log.Printf("live osc resolve: %v", err)
		return
	}
	stop := make(chan struct{})
	s.remoteMu.Lock()
	s.remoteStop = stop
	s.remoteMu.Unlock()
	go x32.StartXremote(s.OSC, addr, 8*time.Second, stop)
	go x32.StartMeters(s.OSC, addr, 8*time.Second, 1, stop) // timeFactor 1 ≈ 50ms meter updates
}

func (s *Server) stopLiveOSC() {
	s.remoteMu.Lock()
	if s.remoteStop != nil {
		close(s.remoteStop)
		s.remoteStop = nil
	}
	s.remoteMu.Unlock()
}

func (s *Server) beginWork(auto bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return false
	}
	if auto {
		switch s.Hub.GlobalPhase() {
		case "ready", "searching":
			return false
		}
	}
	s.busy = true
	return true
}

func (s *Server) endWork() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
