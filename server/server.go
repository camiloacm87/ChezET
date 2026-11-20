// Package server es el núcleo de Ajedrez en Red: un pequeño servidor web local
// que sirve la interfaz, guarda la partida y la sincroniza entre los dos
// jugadores. Lo usan la app de escritorio (Windows/Mac) y, en el futuro, la de
// Android (vía gomobile).
//
// Un jugador "crea" la partida (su equipo actúa de anfitrión) y el otro se
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
	defaultPort   = 8765
	discoveryPort = 8766
	appTag        = "chezzred/1"
	maxChat       = 200
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
	ID        string
	Round     int
	Players   [2]*Player // 0 = blancas, 1 = negras
	Moves     []string   // jugadas en formato UCI (e2e4, e7e8q)
	Result    string     // "", "1-0", "0-1", "1/2-1/2"
	Reason    string
	DrawOffer int // -1 ninguna, si no el color que ofrece
	Rematch   [2]bool
	Chat      []ChatMsg
}

type sub struct {
	ch    chan struct{}
	token string
}

type peer struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Status string `json:"status"`
	seen   time.Time
}

type Server struct {
	mu         sync.Mutex
	port       int
	instanceID string
	hostToken  string
	game       *Game
	subs       map[*sub]struct{}
	peers      map[string]*peer
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
	for i, p := range s.game.Players {
		if p != nil && p.Token == token {
			return i
		}
	}
	return -1
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
		b, _ := json.Marshal(map[string]any{"gameId": ""})
		return b
	}
	var ps [2]playerView
	for i, p := range g.Players {
		if p != nil {
			ps[i] = playerView{p.Name, true, p.Online > 0}
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
	b, _ := json.Marshal(map[string]any{
		"gameId":    g.ID,
		"round":     g.Round,
		"you":       s.seatOf(token),
		"players":   ps,
		"moves":     moves,
		"result":    g.Result,
		"reason":    g.Reason,
		"drawOffer": g.DrawOffer,
		"rematch":   g.Rematch,
		"chat":      chat,
	})
	return b
}

func (s *Server) hostName() string {
	if s.game == nil {
		return ""
	}
	if i := s.seatOf(s.hostToken); i >= 0 {
		return s.game.Players[i].Name
	}
	return ""
}

func (s *Server) status() string {
	if s.game == nil {
		return ""
	}
	if s.game.Players[0] == nil || s.game.Players[1] == nil {
		return "esperando"
	}
	return "en juego"
}

// ---------- Handlers ----------

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	primary, ips := localIPs()
	s.mu.Lock()
	defer s.mu.Unlock()
	free := 0
	gid := ""
	if s.game != nil {
		gid = s.game.ID
		for _, p := range s.game.Players {
			if p == nil {
				free++
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"port":      s.port,
		"primary":   primary,
		"ips":       ips,
		"isLocal":   isLocal(r),
		"hosting":   s.game != nil,
		"gameId":    gid,
		"seatsFree": free,
		"hostName":  s.hostName(),
		"status":    s.status(),
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
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, "petición inválida")
		return
	}
	seat := 0
	switch req.Color {
	case "b":
		seat = 1
	case "r":
		b := make([]byte, 1)
		_, _ = rand.Read(b)
		seat = int(b[0] & 1)
	}
	tok := newToken()
	g := &Game{ID: newToken()[:8], DrawOffer: -1}
	g.Players[seat] = &Player{Name: cleanName(req.Name), Token: tok}
	s.mu.Lock()
	s.game = g
	s.hostToken = tok
	s.notify()
	s.mu.Unlock()
	log.Printf("Partida creada por %q (juega con %s)", cleanName(req.Name), map[int]string{0: "blancas", 1: "negras"}[seat])
	writeJSON(w, 200, map[string]any{"gameId": g.ID, "token": tok})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !isLocal(r) {
		fail(w, 403, "no permitido")
		return
	}
	s.mu.Lock()
	s.game = nil
	s.hostToken = ""
	s.notify()
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
	for i, p := range s.game.Players {
		if p == nil {
			tok := newToken()
			s.game.Players[i] = &Player{Name: cleanName(req.Name), Token: tok}
			s.notify()
			log.Printf("%q se unió a la partida desde %s", cleanName(req.Name), r.RemoteAddr)
			writeJSON(w, 200, map[string]any{"gameId": s.game.ID, "token": tok})
			return
		}
	}
	fail(w, 409, "la partida ya tiene dos jugadores")
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
		me = s.game.Players[i]
		me.Online++
	}
	s.notify()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.subs, sb)
		if me != nil {
			me.Online--
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

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "método no permitido")
		return
	}
	var req struct {
		Token  string `json:"token"`
		Type   string `json:"type"`
		Move   string `json:"move"`
		Ply    int    `json:"ply"`
		Result string `json:"result"`
		Reason string `json:"reason"`
		Text   string `json:"text"`
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
	if me < 0 {
		fail(w, 403, "no eres jugador de esta partida")
		return
	}
	other := 1 - me
	full := g.Players[0] != nil && g.Players[1] != nil

	switch req.Type {
	case "move":
		if !full {
			fail(w, 409, "esperando al oponente")
			return
		}
		if g.Result != "" {
			fail(w, 409, "la partida ya terminó")
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
		g.Moves = append(g.Moves, req.Move)
		if g.DrawOffer == other {
			g.DrawOffer = -1 // jugar rechaza la oferta de tablas del rival
		}
		if req.Result != "" && endReasons[req.Reason] &&
			(req.Result == "1-0" || req.Result == "0-1" || req.Result == "1/2-1/2") {
			g.Result, g.Reason = req.Result, req.Reason
		}
	case "resign":
		if g.Result != "" || !full {
			fail(w, 409, "no se puede abandonar ahora")
			return
		}
		g.Result = map[int]string{0: "0-1", 1: "1-0"}[me]
		g.Reason = "Abandono"
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
		g.Rematch[me] = true
		if g.Rematch[0] && g.Rematch[1] {
			g.Players[0], g.Players[1] = g.Players[1], g.Players[0]
			g.Moves = nil
			g.Result, g.Reason = "", ""
			g.DrawOffer = -1
			g.Rematch = [2]bool{}
			g.Round++
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
		g.Chat = append(g.Chat, ChatMsg{From: g.Players[me].Name, Color: me, Text: t, T: time.Now().Unix()})
		if len(g.Chat) > maxChat {
			g.Chat = g.Chat[len(g.Chat)-maxChat:]
		}
	default:
		fail(w, 400, "acción desconocida")
		return
	}
	s.notify()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
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
	Tag    string `json:"tag"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Port   int    `json:"port"`
	Status string `json:"status"`
	Addr   string `json:"addr"` // IP principal del emisor
}

func (s *Server) discovery() {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: discoveryPort})
	if err != nil {
		log.Printf("Aviso: no se pudo escuchar el puerto de descubrimiento %d (%v). "+
			"Se podrá unir escribiendo la IP manualmente.", discoveryPort, err)
		conn, err = net.ListenUDP("udp4", &net.UDPAddr{})
		if err != nil {
			return
		}
	} else {
		go s.listenBeacons(conn)
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for range tick.C {
		s.mu.Lock()
		var b []byte
		if s.game != nil {
			primary, _ := localIPs()
			b, _ = json.Marshal(beacon{appTag, s.instanceID, s.hostName(), s.port, s.status(), primary})
		}
		s.mu.Unlock()
		if b == nil {
			continue
		}
		for _, a := range broadcastAddrs() {
			_, _ = conn.WriteToUDP(b, &net.UDPAddr{IP: a, Port: discoveryPort})
		}
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
		s.peers[key] = &peer{Name: cleanName(b.Name), Host: host, Port: b.Port, Status: b.Status, seen: time.Now()}
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

// Start levanta el servidor en el puerto indicado (o en uno libre si está
// ocupado) y devuelve la URL local donde se muestra la interfaz.
func Start(port int) (string, error) {
	s := &Server{
		instanceID: newToken(),
		subs:       map[*sub]struct{}{},
		peers:      map[string]*peer{},
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("El puerto %d está ocupado, usando otro libre…", port)
		ln, err = net.Listen("tcp", ":0")
		if err != nil {
			return "", err
		}
	}
	s.port = ln.Addr().(*net.TCPAddr).Port

	static, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(static))
	mux := http.NewServeMux()
	mux.HandleFunc("/api/info", s.handleInfo)
	mux.HandleFunc("/api/create", s.handleCreate)
	mux.HandleFunc("/api/cancel", s.handleCancel)
	mux.HandleFunc("/api/join", s.handleJoin)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/action", s.handleAction)
	mux.HandleFunc("/api/discover", s.handleDiscover)
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFileFS(w, r, static, "game.html")
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	}))

	go s.discovery()
	go func() { log.Println(http.Serve(ln, mux)) }()
	return fmt.Sprintf("http://localhost:%d/", s.port), nil
}

// Addresses devuelve la IP principal del equipo y todas sus IPv4.
func Addresses() (string, []string) { return localIPs() }
