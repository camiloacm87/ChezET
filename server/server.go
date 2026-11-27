// Package server es el núcleo de Ajedrez en Red: un pequeño servidor web local
// que sirve la interfaz, guarda la partida y la sincroniza entre los jugadores
// jugadores. Lo usan la app de escritorio (Windows/Mac) y, en el futuro, la de
// Android (vía gomobile).
//
// Un jugador "crea" la partida (su equipo actúa de anfitrión) y los demás se
// "une" desde la red local, eligiéndola de la lista de partidas descubiertas
// automáticamente o escribiendo la IP.
package server

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFS embed.FS

const (
	defaultPort    = 8765
	discoveryPort  = 8766
	appTag         = "chezzred/1"
	maxChat        = 200
	maxCardPlayers = 8
)

var moveRe = regexp.MustCompile(`^[a-h][1-8][a-h][1-8][qrbn]?$`)

var endReasons = map[string]bool{
	"Jaque mate":                  true,
	"Rey ahogado":                 true,
	"Material insuficiente":       true,
	"Regla de los 50 movimientos": true,
	"Triple repetición":           true,
}

type Player struct {
	Name   string
	Token  string
	Online int
}

type ChatMsg struct {
	From  string `json:"from"`
	Color int    `json:"color"`
	Text  string `json:"text"`
	T     int64  `json:"t"`
}

type Game struct {
	ID           string
	Round        int
	GameType     string
	PlayerLimit  int
	Started      bool       // UNO/Golpeado: el anfitrión ya inició la partida
	Players      [2]*Player // 0 = blancas, 1 = negras
	ExtraPlayers []*Player
	Spectators   map[string]string
	Moves        []string // jugadas en formato UCI (e2e4, e7e8q)
	Result       string   // "", "1-0", "0-1", "1/2-1/2"
	Reason       string
	Checkers     *checkersState
	Cards        *cardGameState
	DrawOffer    int // -1 ninguna, si no el color que ofrece
	Rematch      [2]bool
	Chat         []ChatMsg
	TimeMode     string
	TimeLimit    time.Duration
	Remaining    [2]time.Duration
	TurnStarted  time.Time
	ClockTimer   *time.Timer
}

type sub struct {
	ch    chan struct{}
	token string
}

type peer struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Status   string `json:"status"`
	GameType string `json:"gameType"`
	seen     time.Time
}

// remoteSeat es la partida de otro equipo a la que se unió este jugador; al
// cerrar la app se le avisa al anfitrión.
type remoteSeat struct {
	Base  string `json:"base"`
	Token string `json:"token"`
}

type Server struct {
	mu         sync.Mutex
	handler    http.Handler
	lanWanted  int          // puerto de red configurado (8765 por defecto)
	lan        *http.Server // servidor de red activo; nil = red cerrada
	lanPort    int          // puerto de red abierto (0 = cerrado)
	instanceID string
	hostToken  string
	game       *Game
	endReason  string // por qué terminó la última partida (se muestra a los jugadores)
	remote     *remoteSeat
	subs       map[*sub]struct{}
	peers      map[string]*peer
	udp        *net.UDPConn // escucha de anuncios; solo mientras se buscan partidas
	discoverAt time.Time
}

var current *Server

var androidNetworkMu sync.RWMutex
var androidPrimaryIP string
var androidIPList []string

// SetLocalNetwork permite a una aplicación anfitriona (por ejemplo Android)
// proporcionar las direcciones visibles desde la red Wi-Fi. Es especialmente
// necesario en Android 11+, donde las restricciones del sistema pueden impedir
// que net.Interfaces/net.Dial descubran la IP local como esperan las apps Go.
func SetLocalNetwork(primary string, ipsCSV string) {
	androidNetworkMu.Lock()
	androidPrimaryIP = strings.TrimSpace(primary)
	androidIPList = nil
	for _, ip := range strings.Split(ipsCSV, ",") {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			androidIPList = append(androidIPList, ip)
		}
	}
	if androidPrimaryIP != "" && len(androidIPList) == 0 {
		androidIPList = []string{androidPrimaryIP}
	}
	androidNetworkMu.Unlock()
}

// openLAN abre el puerto de red para que el rival pueda conectarse. Con s.mu tomado.
func (s *Server) openLAN() error {
	if s.lan != nil {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.lanWanted))
	if err != nil {
		log.Printf("El puerto %d está ocupado, usando otro libre…", s.lanWanted)
		if ln, err = net.Listen("tcp", ":0"); err != nil {
			return err
		}
	}
	srv := &http.Server{Handler: s.handler}
	go func() { _ = srv.Serve(ln) }()
	s.lan = srv
	s.lanPort = ln.Addr().(*net.TCPAddr).Port
	log.Printf("Red abierta en el puerto %d", s.lanPort)
	return nil
}

// closeLAN cierra el puerto de red (tras un instante, para que el último aviso
// llegue a los jugadores conectados). Con s.mu tomado.
func (s *Server) closeLAN() {
	if s.lan == nil {
		return
	}
	srv := s.lan
	s.lan, s.lanPort = nil, 0
	go func() {
		time.Sleep(800 * time.Millisecond)
		_ = srv.Close()
		log.Printf("Red cerrada")
	}()
}

// endGame termina la partida actual, avisa a los jugadores con el motivo y
// cierra la red. Con s.mu tomado.
func (s *Server) endGame(reason string) {
	if s.game == nil {
		return
	}
	log.Printf("Partida terminada: %s", reason)
	if s.game.ClockTimer != nil {
		s.game.ClockTimer.Stop()
	}
	s.game = nil
	s.hostToken = ""
	s.endReason = reason
	s.notify()
	s.closeLAN()
}

func newToken() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isLocal(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16<<10)).Decode(v)
}

func cleanName(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 24 {
		s = string(r[:24])
	}
	if s == "" {
		s = "Jugador"
	}
	return s
}

// notify despierta a todos los suscriptores SSE. Debe llamarse con s.mu tomado.
func (s *Server) notify() {
	for sb := range s.subs {
		select {
		case sb.ch <- struct{}{}:
		default:
		}
	}
}

func (s *Server) seatOf(token string) int {
	if s.game == nil || token == "" {
		return -1
	}
	for i, p := range playerSeats(s.game) {
		if p != nil && p.Token == token {
			return i
		}
	}
	return -1
}

func playerSeats(g *Game) []*Player {
	if g.ExtraPlayers != nil {
		return g.ExtraPlayers
	}
	return g.Players[:]
}

func isCardGame(g *Game) bool { return g.ExtraPlayers != nil }

func playersReady(g *Game) bool {
	if isCardGame(g) {
		return g.Started
	}
	seats := playerSeats(g)
	if len(seats) == 0 {
		return false
	}
	for _, p := range seats {
		if p == nil {
			return false
		}
	}
	return true
}

func (s *Server) startClockLocked(g *Game) {
	if g.TimeMode == "" || g.TimeMode == "none" || g.Result != "" ||
		!playersReady(g) {
		return
	}
	if isCardGame(g) {
		s.startCardClockLocked(g)
		return
	}
	if g.ClockTimer != nil {
		g.ClockTimer.Stop()
	}
	g.TurnStarted = time.Now()
	turn := clockTurn(g)
	duration := g.TimeLimit
	if g.TimeMode == "total" {
		duration = g.Remaining[turn]
	} else {
		g.Remaining[turn] = g.TimeLimit
	}
	started := g.TurnStarted
	g.ClockTimer = time.AfterFunc(duration, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.game != g || g.Result != "" || !g.TurnStarted.Equal(started) {
			return
		}
		s.expireClockLocked(g)
		s.notify()
	})
}

// startCardClockLocked arranca el tiempo del turno actual en UNO/Golpeado. Si se
// agota, el servidor juega por el jugador. Con s.mu tomado.
func (s *Server) startCardClockLocked(g *Game) {
	if g.ClockTimer != nil {
		g.ClockTimer.Stop()
	}
	g.TurnStarted = time.Now()
	started := g.TurnStarted
	g.ClockTimer = time.AfterFunc(g.TimeLimit, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.game != g || g.Result != "" || !g.TurnStarted.Equal(started) {
			return
		}
		s.cardTimeoutLocked(g)
		s.notify()
	})
}

// cardTimeoutLocked juega automáticamente el turno agotado y reinicia el reloj.
func (s *Server) cardTimeoutLocked(g *Game) {
	autoCardTurn(g)
	if g.Result == "" {
		s.startCardClockLocked(g)
	} else {
		g.TurnStarted = time.Time{}
	}
}

// clockTurn devuelve quién tiene el turno en los juegos de dos (0 o 1).
func clockTurn(g *Game) int {
	if g.Checkers != nil {
		return g.Checkers.Turn
	}
	return len(g.Moves) % 2
}

func (s *Server) expireClockLocked(g *Game) {
	if g.ClockTimer != nil {
		g.ClockTimer.Stop()
	}
	turn := clockTurn(g)
	g.Remaining[turn] = 0
	if g.GameType == "checkers" {
		g.Result = fmt.Sprintf("winner:%d", 1-turn)
	} else {
		g.Result = map[int]string{0: "0-1", 1: "1-0"}[turn]
	}
	g.Reason = "Tiempo agotado"
}

func (s *Server) stopClockLocked(g *Game) {
	if g.ClockTimer != nil {
		g.ClockTimer.Stop()
	}
	if isCardGame(g) {
		g.TurnStarted = time.Time{}
		return
	}
	if g.TurnStarted.IsZero() {
		return
	}
	turn := clockTurn(g)
	available := g.TimeLimit
	if g.TimeMode == "total" {
		available = g.Remaining[turn]
	}
	left := available - time.Since(g.TurnStarted)
	if left < 0 {
		left = 0
	}
	g.Remaining[turn] = left
	g.TurnStarted = time.Time{}
}

func (s *Server) clockExpiredLocked(g *Game) bool {
	if g.TimeMode == "" || g.TimeMode == "none" || g.TurnStarted.IsZero() || g.Result != "" {
		return false
	}
	if isCardGame(g) {
		if time.Since(g.TurnStarted) < g.TimeLimit {
			return false
		}
		s.cardTimeoutLocked(g)
		return true
	}
	turn := clockTurn(g)
	available := g.TimeLimit
	if g.TimeMode == "total" {
		available = g.Remaining[turn]
	}
	if time.Since(g.TurnStarted) < available {
		return false
	}
	s.expireClockLocked(g)
	return true
}

type playerView struct {
	Name   string `json:"name"`
	Joined bool   `json:"joined"`
	Online bool   `json:"online"`
}

// viewJSON serializa el estado visto por el poseedor de token. Con s.mu tomado.
func (s *Server) viewJSON(token string) []byte {
	g := s.game
	if g == nil {
		b, _ := json.Marshal(map[string]any{"gameId": "", "ended": s.endReason})
		return b
	}
	seats := playerSeats(g)
	players := make([]playerView, len(seats))
	for i, p := range seats {
		if p != nil {
			players[i] = playerView{p.Name, true, p.Online > 0}
		}
	}
	moves := g.Moves
	if moves == nil {
		moves = []string{}
	}
	chat := g.Chat
	if chat == nil {
		chat = []ChatMsg{}
	}
	state := map[string]any{
		"gameId":      g.ID,
		"round":       g.Round,
		"gameType":    g.GameType,
		"you":         s.seatOf(token),
		"players":     players,
		"started":     playersReady(g),
		"maxPlayers":  g.PlayerLimit,
		"moves":       moves,
		"result":      g.Result,
		"reason":      g.Reason,
		"drawOffer":   g.DrawOffer,
		"rematch":     g.Rematch,
		"chat":        chat,
		"timeMode":    g.TimeMode,
		"timeLimitMs": g.TimeLimit.Milliseconds(),
		"clocks":      s.clockView(g),
	}
	if g.Checkers != nil {
		state["checkers"] = checkersView(g.Checkers)
	}
	if g.Cards != nil && playersReady(g) {
		state["cards"] = cardGameView(g.Cards, s.seatOf(token))
		if g.TimeMode == "per-move" && g.Result == "" && !g.TurnStarted.IsZero() {
			left := g.TimeLimit - time.Since(g.TurnStarted)
			if left < 0 {
				left = 0
			}
			state["turnLeftMs"] = left.Milliseconds()
		}
	}
	b, _ := json.Marshal(state)
	return b
}

func (s *Server) clockView(g *Game) [2]int64 {
	clocks := [2]int64{g.Remaining[0].Milliseconds(), g.Remaining[1].Milliseconds()}
	if g.Result == "" && !g.TurnStarted.IsZero() && g.TimeMode != "" && g.TimeMode != "none" {
		turn := clockTurn(g)
		left := clocks[turn] - time.Since(g.TurnStarted).Milliseconds()
		if left < 0 {
			left = 0
		}
		clocks[turn] = left
	}
	return clocks
}

func (s *Server) hostName() string {
	if s.game == nil {
		return ""
	}
	if i := s.seatOf(s.hostToken); i >= 0 {
		return playerSeats(s.game)[i].Name
	}
	return ""
}

func (s *Server) status() string {
	if s.game == nil {
		return ""
	}
	if !playersReady(s.game) {
		return "esperando"
	}
	return "en juego"
}

// ---------- Handlers ----------

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	primary, ips := localIPs()
	s.mu.Lock()
	defer s.mu.Unlock()
	free := 0
	gid := ""
	gameType := ""
	if s.game != nil {
		gid = s.game.ID
		gameType = s.game.GameType
		if isCardGame(s.game) {
			if !s.game.Started {
				free = s.game.PlayerLimit - len(s.game.ExtraPlayers)
			}
		} else {
			for _, p := range playerSeats(s.game) {
				if p == nil {
					free++
				}
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"port":      s.lanPort, // 0 = red cerrada
		"primary":   primary,
		"ips":       ips,
		"isLocal":   isLocal(r),
		"hosting":   s.game != nil,
		"gameId":    gid,
		"seatsFree": free,
		"hostName":  s.hostName(),
		"status":    s.status(),
		"gameType":  gameType,
	})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "método no permitido")
		return
	}
	if !isLocal(r) {
		fail(w, 403, "solo el anfitrión puede crear partidas")
		return
	}
	var req struct {
		Name             string `json:"name"`
		Color            string `json:"color"`
		GameType         string `json:"gameType"`
		TimeMode         string `json:"timeMode"`
		TimeLimitSeconds int    `json:"timeLimitSeconds"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, "petición inválida")
		return
	}
	if req.GameType == "" {
		req.GameType = "chess"
	}
	if req.GameType != "chess" && req.GameType != "checkers" &&
		req.GameType != "uno" && req.GameType != "golpeado" {
		fail(w, 400, "juego desconocido")
		return
	}
	playerLimit := 2
	if req.GameType == "uno" || req.GameType == "golpeado" {
		playerLimit = maxCardPlayers
	}
	if req.TimeMode == "" {
		req.TimeMode = "none"
	}
	if req.TimeMode != "none" && req.TimeMode != "per-move" && req.TimeMode != "total" {
		fail(w, 400, "configuración de tiempo inválida")
		return
	}
	if (req.GameType == "uno" || req.GameType == "golpeado") && req.TimeMode == "total" {
		fail(w, 400, "este juego solo admite tiempo por turno")
		return
	}
	if req.TimeMode != "none" && (req.TimeLimitSeconds < 5 || req.TimeLimitSeconds > 3600) {
		fail(w, 400, "el tiempo debe estar entre 5 segundos y 60 minutos")
		return
	}
	seat := 0
	if req.GameType == "chess" || req.GameType == "checkers" {
		switch req.Color {
		case "b":
			seat = 1
		case "r":
			b := make([]byte, 1)
			_, _ = rand.Read(b)
			seat = int(b[0] & 1)
		}
	}
	tok := newToken()
	limit := time.Duration(req.TimeLimitSeconds) * time.Second
	g := &Game{ID: newToken()[:8], GameType: req.GameType, PlayerLimit: playerLimit,
		DrawOffer: -1, TimeMode: req.TimeMode, TimeLimit: limit}
	if req.GameType == "uno" || req.GameType == "golpeado" {
		g.ExtraPlayers = make([]*Player, 0, playerLimit)
	}
	if req.TimeMode != "none" {
		g.Remaining = [2]time.Duration{limit, limit}
	}
	if isCardGame(g) {
		g.ExtraPlayers = append(g.ExtraPlayers, &Player{Name: cleanName(req.Name), Token: tok})
	} else {
		playerSeats(g)[seat] = &Player{Name: cleanName(req.Name), Token: tok}
	}
	if req.GameType == "checkers" {
		g.Checkers = newCheckersState()
	}
	s.mu.Lock()
	if err := s.openLAN(); err != nil {
		s.mu.Unlock()
		fail(w, 500, "no se pudo abrir la red: "+err.Error())
		return
	}
	s.game = g
	s.hostToken = tok
	s.endReason = ""
	s.notify()
	s.mu.Unlock()
	log.Printf("Partida de %s creada por %q", req.GameType, cleanName(req.Name))
	writeJSON(w, 200, map[string]any{"gameId": g.ID, "token": tok})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !isLocal(r) {
		fail(w, 403, "no permitido")
		return
	}
	s.mu.Lock()
	s.endGame("El anfitrión canceló la partida")
	s.mu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleRemote guarda (o borra, con token vacío) la partida remota a la que se
// unió este jugador, para avisar al anfitrión si se cierra la app.
func (s *Server) handleRemote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !isLocal(r) {
		fail(w, 403, "no permitido")
		return
	}
	var req remoteSeat
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, "petición inválida")
		return
	}
	s.mu.Lock()
	if req.Token == "" || req.Base == "" {
		s.remote = nil
	} else {
		s.remote = &req
	}
	s.mu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "método no permitido")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, "petición inválida")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.game == nil {
		fail(w, 404, "no hay partida creada")
		return
	}
	if isCardGame(s.game) {
		if s.game.Started {
			fail(w, 409, "la partida ya comenzó")
			return
		}
		if len(s.game.ExtraPlayers) >= s.game.PlayerLimit {
			fail(w, 409, "la partida está completa")
			return
		}
		tok := newToken()
		s.game.ExtraPlayers = append(s.game.ExtraPlayers, &Player{Name: cleanName(req.Name), Token: tok})
		s.notify()
		log.Printf("%q se unió a la partida desde %s", cleanName(req.Name), r.RemoteAddr)
		writeJSON(w, 200, map[string]any{"gameId": s.game.ID, "token": tok})
		return
	}
	seats := playerSeats(s.game)
	for i, p := range seats {
		if p == nil {
			tok := newToken()
			seats[i] = &Player{Name: cleanName(req.Name), Token: tok}
			if playersReady(s.game) {
				s.startClockLocked(s.game)
			}
			s.notify()
			log.Printf("%q se unió a la partida desde %s", cleanName(req.Name), r.RemoteAddr)
			writeJSON(w, 200, map[string]any{"gameId": s.game.ID, "token": tok})
			return
		}
	}
	fail(w, 409, "la partida está completa")
}

func (s *Server) handleWatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "método no permitido")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, "petición inválida")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.game == nil {
		fail(w, 404, "no hay partida creada")
		return
	}
	if !playersReady(s.game) {
		fail(w, 409, "la partida todavía no está llena")
		return
	}
	if s.game.Spectators == nil {
		s.game.Spectators = make(map[string]string)
	}
	token := newToken()
	s.game.Spectators[token] = cleanName(req.Name)
	writeJSON(w, 200, map[string]any{"gameId": s.game.ID, "token": token})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	b := s.viewJSON(r.URL.Query().Get("token"))
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "streaming no soportado")
		return
	}
	token := r.URL.Query().Get("token")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	sb := &sub{ch: make(chan struct{}, 1), token: token}
	s.mu.Lock()
	s.subs[sb] = struct{}{}
	var me *Player
	if i := s.seatOf(token); i >= 0 {
		me = playerSeats(s.game)[i]
		me.Online++
	}
	s.notify()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.subs, sb)
		if me != nil {
			me.Online--
			if me.Online == 0 {
				s.watchGuest(me)
			}
		}
		s.notify()
		s.mu.Unlock()
	}()

	fmt.Fprint(w, "retry: 2000\n\n")
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		case <-sb.ch:
			s.mu.Lock()
			b := s.viewJSON(token)
			s.mu.Unlock()
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

// removeWaitingPlayer quita a un jugador de la sala de espera de UNO/Golpeado.
// Con s.mu tomado.
func (s *Server) removeWaitingPlayer(token string) {
	g := s.game
	for i, p := range g.ExtraPlayers {
		if p != nil && p.Token == token {
			g.ExtraPlayers = append(g.ExtraPlayers[:i], g.ExtraPlayers[i+1:]...)
			return
		}
	}
}

// guestGrace es cuánto se espera a que el invitado vuelva a conectarse (p. ej.
// al recargar la página) antes de dar la partida por abandonada.
var guestGrace = 10 * time.Second

// watchGuest termina la partida si el invitado sigue desconectado pasado el
// margen (cerró la app sin avisar o perdió la red). Con s.mu tomado.
func (s *Server) watchGuest(p *Player) {
	if p.Token == s.hostToken {
		return // el anfitrión es este mismo equipo; su cierre lo gestiona Shutdown
	}
	time.AfterFunc(guestGrace, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if p.Online == 0 && s.seatOf(p.Token) >= 0 {
			if s.game != nil && isCardGame(s.game) && !s.game.Started {
				s.removeWaitingPlayer(p.Token) // en la sala de espera solo se quita su lugar
				s.notify()
				return
			}
			reason := "Tu rival cerró el juego o perdió la conexión"
			if s.game != nil && s.game.ExtraPlayers != nil {
				reason = "Un jugador cerró el juego o perdió la conexión"
			}
			s.endGame(reason)
		}
	})
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "método no permitido")
		return
	}
	var req struct {
		Token     string `json:"token"`
		Type      string `json:"type"`
		Move      string `json:"move"`
		Ply       int    `json:"ply"`
		Result    string `json:"result"`
		Reason    string `json:"reason"`
		Text      string `json:"text"`
		Action    string `json:"action"`
		From      int    `json:"from"`
		To        int    `json:"to"`
		Card      int    `json:"card"`
		Color     string `json:"color"`
		Cards     []int  `json:"cards"`
		MeldIndex int    `json:"meldIndex"`
		Uno       bool   `json:"uno"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, "petición inválida")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.game
	if g == nil {
		fail(w, 404, "no hay partida")
		return
	}
	me := s.seatOf(req.Token)
	spectatorName, isSpectator := g.Spectators[req.Token]
	if me < 0 && !(req.Type == "chat" && isSpectator && req.Token != "") {
		fail(w, 403, "no eres jugador de esta partida")
		return
	}
	other := -1
	if me >= 0 {
		other = (me + 1) % len(playerSeats(g))
	}
	full := playersReady(g)
	if s.clockExpiredLocked(g) {
		s.notify()
		fail(w, 409, "se agotó el tiempo")
		return
	}

	switch req.Type {
	case "leave":
		if req.Token == s.hostToken {
			s.endGame("El anfitrión cerró la partida")
		} else if isCardGame(g) && !g.Started {
			s.removeWaitingPlayer(req.Token)
			s.notify()
		} else if g.ExtraPlayers != nil {
			s.endGame("Un jugador salió de la partida")
		} else {
			s.endGame("Tu rival salió de la partida")
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	case "move":
		if !full {
			fail(w, 409, "esperando al oponente")
			return
		}
		if g.Result != "" {
			fail(w, 409, "la partida ya terminó")
			return
		}
		if g.GameType == "checkers" {
			turnBefore := g.Checkers.Turn
			if err := makeCheckersMove(g, me, req.From, req.To); err != nil {
				fail(w, 409, err.Error())
				return
			}
			if g.Checkers.Turn != turnBefore || g.Result != "" { // una captura múltiple no cambia el reloj
				if !g.TurnStarted.IsZero() {
					available := g.TimeLimit
					if g.TimeMode == "total" {
						available = g.Remaining[turnBefore]
					}
					g.Remaining[turnBefore] = max(0, available-time.Since(g.TurnStarted))
				}
				if g.Result == "" {
					s.startClockLocked(g)
				} else {
					if g.ClockTimer != nil {
						g.ClockTimer.Stop()
					}
					g.TurnStarted = time.Time{}
				}
			}
			s.notify()
			writeJSON(w, 200, map[string]any{"ok": true})
			return
		}
		if len(g.Moves)%2 != me {
			fail(w, 409, "no es tu turno")
			return
		}
		if req.Ply != len(g.Moves) {
			fail(w, 409, "jugada desfasada")
			return
		}
		if !moveRe.MatchString(req.Move) {
			fail(w, 400, "jugada inválida")
			return
		}
		turn := len(g.Moves) % 2
		if !g.TurnStarted.IsZero() {
			available := g.TimeLimit
			if g.TimeMode == "total" {
				available = g.Remaining[turn]
			}
			g.Remaining[turn] = available - time.Since(g.TurnStarted)
			if g.Remaining[turn] < 0 {
				g.Remaining[turn] = 0
			}
		}
		g.Moves = append(g.Moves, req.Move)
		if g.DrawOffer == other {
			g.DrawOffer = -1 // jugar rechaza la oferta de tablas del rival
		}
		if req.Result != "" && endReasons[req.Reason] &&
			(req.Result == "1-0" || req.Result == "0-1" || req.Result == "1/2-1/2") {
			g.Result, g.Reason = req.Result, req.Reason
		}
		if g.Result == "" {
			s.startClockLocked(g)
		} else {
			g.TurnStarted = time.Time{}
		}
	case "resign":
		if g.Result != "" || !full {
			fail(w, 409, "no se puede abandonar ahora")
			return
		}
		if g.GameType == "chess" {
			g.Result = map[int]string{0: "0-1", 1: "1-0"}[me]
		} else {
			g.Result = fmt.Sprintf("winner:%d", other)
		}
		g.Reason = "Abandono"
	case "start":
		if !isCardGame(g) || req.Token != s.hostToken {
			fail(w, 403, "solo el anfitrión puede iniciar la partida")
			return
		}
		if g.Started {
			fail(w, 409, "la partida ya comenzó")
			return
		}
		if len(g.ExtraPlayers) < 2 {
			fail(w, 409, "se necesitan al menos 2 jugadores")
			return
		}
		if g.GameType == "uno" {
			g.Cards = newUnoState(len(g.ExtraPlayers))
		} else {
			g.Cards = newGolpeadoState(len(g.ExtraPlayers))
		}
		g.Started = true
		s.startClockLocked(g)
	case "game-action":
		if !full || g.Result != "" {
			fail(w, 409, "la partida no está disponible para jugar")
			return
		}
		turnsBefore := g.Cards.TurnCount
		if err := makeCardAction(g, me, req.Action, req.Card, req.Color, req.Cards, req.MeldIndex, req.Uno); err != nil {
			fail(w, 409, err.Error())
			return
		}
		if g.Result == "" && g.Cards.TurnCount != turnsBefore {
			s.startClockLocked(g) // turno nuevo: el reloj vuelve a empezar
		}
	case "draw-offer":
		if g.Result != "" || !full {
			fail(w, 409, "no se puede ofrecer tablas ahora")
			return
		}
		g.DrawOffer = me
	case "draw-accept":
		if g.Result != "" || g.DrawOffer != other {
			fail(w, 409, "no hay oferta de tablas")
			return
		}
		g.Result, g.Reason, g.DrawOffer = "1/2-1/2", "Tablas de mutuo acuerdo", -1
	case "draw-decline":
		if g.DrawOffer == other {
			g.DrawOffer = -1
		}
	case "rematch":
		if g.Result == "" {
			fail(w, 409, "la partida sigue en juego")
			return
		}
		if g.ExtraPlayers != nil {
			fail(w, 409, "crea una nueva partida para jugar otra ronda")
			return
		}
		g.Rematch[me] = true
		if g.Rematch[0] && g.Rematch[1] {
			g.Players[0], g.Players[1] = g.Players[1], g.Players[0]
			g.Moves = nil
			g.Result, g.Reason = "", ""
			if g.GameType == "checkers" {
				g.Checkers = newCheckersState()
			}
			g.DrawOffer = -1
			g.Rematch = [2]bool{}
			g.Round++
			g.TurnStarted = time.Time{}
			if g.TimeMode != "" && g.TimeMode != "none" {
				g.Remaining = [2]time.Duration{g.TimeLimit, g.TimeLimit}
			}
			s.startClockLocked(g)
		}
	case "chat":
		t := strings.TrimSpace(req.Text)
		if rr := []rune(t); len(rr) > 300 {
			t = string(rr[:300])
		}
		if t == "" {
			fail(w, 400, "mensaje vacío")
			return
		}
		from, color := spectatorName, -1
		if me >= 0 {
			from, color = playerSeats(g)[me].Name, me
		}
		g.Chat = append(g.Chat, ChatMsg{From: from, Color: color, Text: t, T: time.Now().Unix()})
		if len(g.Chat) > maxChat {
			g.Chat = g.Chat[len(g.Chat)-maxChat:]
		}
	default:
		fail(w, 400, "acción desconocida")
		return
	}
	if g.Result != "" && req.Type != "move" {
		s.stopClockLocked(g)
	} else if g.Result != "" && g.ClockTimer != nil {
		g.ClockTimer.Stop()
	}
	s.notify()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) {
		fail(w, 403, "no permitido")
		return
	}
	s.mu.Lock()
	s.listenForGames()
	out := []peer{}
	for k, p := range s.peers {
		if time.Since(p.seen) > 7*time.Second {
			delete(s.peers, k)
			continue
		}
		out = append(out, *p)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	writeJSON(w, 200, out)
}

// ---------- Descubrimiento por broadcast UDP ----------

type beacon struct {
	Tag      string `json:"tag"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Status   string `json:"status"`
	Addr     string `json:"addr"` // IP principal del emisor
	GameType string `json:"gameType"`
}

// listenForGames abre (si no lo está) la escucha de anuncios de partidas. Se
// llama mientras la pantalla de inicio busca partidas; discovery la cierra
// cuando deja de usarse. Con s.mu tomado.
func (s *Server) listenForGames() {
	s.discoverAt = time.Now()
	if s.udp != nil {
		return
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: discoveryPort})
	if err != nil {
		log.Printf("Aviso: no se pudo escuchar el puerto de descubrimiento %d (%v). "+
			"Se podrá unir escribiendo la IP manualmente.", discoveryPort, err)
		s.discoverAt = time.Now().Add(30 * time.Second) // no reintentar en cada consulta
		return
	}
	s.udp = conn
	go s.listenBeacons(conn)
}

// discovery anuncia la partida en la red solo mientras se es anfitrión, y
// cierra la escucha de anuncios cuando nadie está buscando partidas.
func (s *Server) discovery() {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for range tick.C {
		s.mu.Lock()
		if s.udp != nil && time.Since(s.discoverAt) > 8*time.Second {
			_ = s.udp.Close()
			s.udp = nil
		}
		var b []byte
		if s.game != nil && s.lanPort != 0 {
			primary, _ := localIPs()
			b, _ = json.Marshal(beacon{
				Tag: appTag, ID: s.instanceID, Name: s.hostName(), Port: s.lanPort,
				Status: s.status(), Addr: primary, GameType: s.game.GameType,
			})
		}
		s.mu.Unlock()
		if b == nil {
			continue
		}
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{}) // solo para enviar
		if err != nil {
			continue
		}
		for _, a := range broadcastAddrs() {
			_, _ = conn.WriteToUDP(b, &net.UDPAddr{IP: a, Port: discoveryPort})
		}
		_ = conn.Close()
	}
}

func (s *Server) listenBeacons(conn *net.UDPConn) {
	buf := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		var b beacon
		if json.Unmarshal(buf[:n], &b) != nil || b.Tag != appTag || b.ID == s.instanceID {
			continue
		}
		// Un mismo equipo puede llegar por varias tarjetas de red: se agrupa por
		// instancia y se prefiere la IP principal que anuncia el emisor.
		host := addr.IP.String()
		key := fmt.Sprintf("%s:%d", b.ID, b.Port)
		s.mu.Lock()
		if old, ok := s.peers[key]; ok && host != b.Addr && time.Since(old.seen) < 7*time.Second {
			host = old.Host
		}
		s.peers[key] = &peer{
			Name: cleanName(b.Name), Host: host, Port: b.Port, Status: b.Status,
			GameType: b.GameType, seen: time.Now(),
		}
		s.mu.Unlock()
	}
}

func broadcastAddrs() []net.IP {
	out := []net.IP{net.IPv4bcast}
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || ifc.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil || len(ipn.Mask) != net.IPv4len {
				continue
			}
			bc := make(net.IP, 4)
			for i := range bc {
				bc[i] = ip4[i] | ^ipn.Mask[i]
			}
			out = append(out, bc)
		}
	}
	return out
}

// localIPs devuelve la IP principal (la que se usaría para salir a la red) y
// todas las IPv4 del equipo.
func localIPs() (string, []string) {
	androidNetworkMu.RLock()
	if androidPrimaryIP != "" || len(androidIPList) > 0 {
		primary := androidPrimaryIP
		ips := append([]string(nil), androidIPList...)
		androidNetworkMu.RUnlock()
		if primary == "" && len(ips) > 0 {
			primary = ips[0]
		}
		return primary, ips
	}
	androidNetworkMu.RUnlock()

	primary := ""
	if c, err := net.Dial("udp4", "10.255.255.255:9"); err == nil { // no envía paquetes
		primary = c.LocalAddr().(*net.UDPAddr).IP.String()
		c.Close()
	}
	ips := []string{}
	if primary != "" && !strings.HasPrefix(primary, "127.") {
		ips = append(ips, primary)
	}
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			ip := ipn.IP.String()
			if ip != primary {
				ips = append(ips, ip)
			}
		}
	}
	if primary == "" && len(ips) > 0 {
		primary = ips[0]
	}
	return primary, ips
}

// cors permite que la pantalla de inicio de otro equipo (otro origen) consulte
// la partida y se una a ella.
func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

// Start levanta el servidor y devuelve la URL local donde se muestra la interfaz.
//
// Al arrancar solo se escucha dentro del propio equipo (127.0.0.1, puerto
// aleatorio): el equipo no es accesible desde la red. El puerto de red
// (lanPort, 8765 por defecto) se abre únicamente al crear una partida y se
// cierra al terminarla o al cerrar la app (Shutdown).
func Start(lanPort int) (string, error) {
	s := &Server{
		lanWanted:  lanPort,
		instanceID: newToken(),
		subs:       map[*sub]struct{}{},
		peers:      map[string]*peer{},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	localPort := ln.Addr().(*net.TCPAddr).Port

	static, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(static))
	mux := http.NewServeMux()
	mux.HandleFunc("/api/info", cors(s.handleInfo))
	mux.HandleFunc("/api/join", cors(s.handleJoin))
	mux.HandleFunc("/api/watch", cors(s.handleWatch))
	mux.HandleFunc("/api/state", cors(s.handleState))
	mux.HandleFunc("/api/events", cors(s.handleEvents))
	mux.HandleFunc("/api/action", cors(s.handleAction))
	mux.HandleFunc("/api/create", s.handleCreate)
	mux.HandleFunc("/api/cancel", s.handleCancel)
	mux.HandleFunc("/api/remote", s.handleRemote)
	mux.HandleFunc("/api/discover", s.handleDiscover)
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFileFS(w, r, static, "game.html")
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	}))
	s.handler = mux
	current = s

	go s.discovery()
	go func() { log.Println(http.Serve(ln, mux)) }()
	return fmt.Sprintf("http://127.0.0.1:%d/", localPort), nil
}

// Shutdown se llama al cerrar la app: si este equipo es anfitrión, avisa a los
// jugadores de que la partida terminó y cierra la red; si está jugando en la
// partida de otro equipo, avisa a ese anfitrión de que se va.
func Shutdown() {
	s := current
	if s == nil {
		return
	}
	s.mu.Lock()
	remote := s.remote
	s.remote = nil
	hosting := s.game != nil
	s.endGame("El anfitrión cerró el juego")
	if s.udp != nil {
		_ = s.udp.Close()
		s.udp = nil
	}
	s.mu.Unlock()

	if remote != nil {
		body, _ := json.Marshal(map[string]string{"token": remote.Token, "type": "leave"})
		c := &http.Client{Timeout: 1500 * time.Millisecond}
		if resp, err := c.Post(remote.Base+"/api/action", "application/json", strings.NewReader(string(body))); err == nil {
			resp.Body.Close()
		}
	}
	if hosting {
		time.Sleep(500 * time.Millisecond) // deja salir el último aviso
	}
}

// Addresses devuelve la IP principal del equipo y todas sus IPv4.
func Addresses() (string, []string) { return localIPs() }
