package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func portOpen(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func post(t *testing.T, url string, body any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 {
		t.Fatalf("POST %s: %d %v", url, resp.StatusCode, out)
	}
	return out
}

// events abre un flujo SSE y devuelve un canal con cada estado recibido.
func events(t *testing.T, url string) (<-chan map[string]any, func()) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan map[string]any, 32)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "data: ") {
				var v map[string]any
				if json.Unmarshal([]byte(line[6:]), &v) == nil {
					ch <- v
				}
			}
		}
	}()
	return ch, func() { resp.Body.Close() }
}

func waitEnded(t *testing.T, ch <-chan map[string]any, want string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				t.Fatalf("el flujo se cerró sin recibir el aviso %q", want)
			}
			if v["gameId"] == "" {
				if v["ended"] != want {
					t.Fatalf("aviso = %q, se esperaba %q", v["ended"], want)
				}
				return
			}
		case <-deadline:
			t.Fatalf("no llegó el aviso %q", want)
		}
	}
}

func startGame(t *testing.T) (local, lanURL string, lan int, hostTok, guestTok string) {
	t.Helper()
	lan = freePort(t)
	local, err := Start(lan)
	if err != nil {
		t.Fatal(err)
	}
	if portOpen(lan) {
		t.Fatal("la red está abierta antes de crear la partida")
	}
	hostTok = post(t, local+"api/create", map[string]string{"name": "Ana", "color": "w"})["token"].(string)
	if !portOpen(lan) {
		t.Fatal("la red no se abrió al crear la partida")
	}
	lanURL = fmt.Sprintf("http://127.0.0.1:%d/", lan)
	guestTok = post(t, lanURL+"api/join", map[string]string{"name": "Luis"})["token"].(string)
	return
}

func assertClosedSoon(t *testing.T, port int) {
	t.Helper()
	for i := 0; i < 30; i++ {
		if !portOpen(port) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("la red sigue abierta")
}

// El anfitrión cierra la app: el invitado recibe el aviso y se cierra la red.
func TestHostShutdownNotifiesGuestAndClosesNetwork(t *testing.T) {
	_, lanURL, lan, _, guestTok := startGame(t)
	ch, stop := events(t, lanURL+"api/events?token="+guestTok)
	defer stop()
	go Shutdown()
	waitEnded(t, ch, "El anfitrión cerró el juego")
	assertClosedSoon(t, lan)
}

// El invitado pulsa "Salir de la partida": el anfitrión recibe el aviso.
func TestGuestLeaveNotifiesHost(t *testing.T) {
	local, lanURL, lan, hostTok, guestTok := startGame(t)
	ch, stop := events(t, local+"api/events?token="+hostTok)
	defer stop()
	post(t, lanURL+"api/action", map[string]string{"token": guestTok, "type": "leave"})
	waitEnded(t, ch, "Tu rival salió de la partida")
	assertClosedSoon(t, lan)
}

// El invitado cierra la app sin avisar (o pierde la red): tras el margen, el
// anfitrión recibe el aviso.
func TestGuestDisconnectEndsGame(t *testing.T) {
	old := guestGrace
	guestGrace = 300 * time.Millisecond
	defer func() { guestGrace = old }()
	local, lanURL, lan, hostTok, guestTok := startGame(t)
	hostCh, stopHost := events(t, local+"api/events?token="+hostTok)
	defer stopHost()
	_, stopGuest := events(t, lanURL+"api/events?token="+guestTok)
	time.Sleep(200 * time.Millisecond)
	stopGuest() // el invitado desaparece
	waitEnded(t, hostCh, "Tu rival cerró el juego o perdió la conexión")
	assertClosedSoon(t, lan)
}

// Un invitado que recarga la página (vuelve antes del margen) no termina la partida.
func TestGuestReloadKeepsGame(t *testing.T) {
	old := guestGrace
	guestGrace = 600 * time.Millisecond
	defer func() { guestGrace = old }()
	_, lanURL, lan, _, guestTok := startGame(t)
	_, stop1 := events(t, lanURL+"api/events?token="+guestTok)
	time.Sleep(100 * time.Millisecond)
	stop1()
	time.Sleep(100 * time.Millisecond)
	_, stop2 := events(t, lanURL+"api/events?token="+guestTok)
	defer stop2()
	time.Sleep(900 * time.Millisecond)
	if !portOpen(lan) {
		t.Fatal("la partida terminó aunque el invitado volvió a conectarse")
	}
}

// El jugador que se unió a otro equipo cierra su app: se avisa a ese anfitrión.
func TestShutdownNotifiesRemoteHost(t *testing.T) {
	got := make(chan string, 1)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		got <- r.URL.Path + " " + req["type"] + " " + req["token"]
	}))
	defer fake.Close()
	local, err := Start(freePort(t))
	if err != nil {
		t.Fatal(err)
	}
	post(t, local+"api/remote", map[string]string{"base": fake.URL, "token": "abc"})
	Shutdown()
	select {
	case v := <-got:
		if v != "/api/action leave abc" {
			t.Fatalf("aviso al anfitrión = %q", v)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no se avisó al anfitrión")
	}
}

// La interfaz solo escucha en el propio equipo.
func TestLocalListenerIsLoopbackOnly(t *testing.T) {
	local, err := Start(freePort(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(local, "http://127.0.0.1:") {
		t.Fatalf("la interfaz no escucha solo en 127.0.0.1: %s", local)
	}
}

func TestSpectatorCanComment(t *testing.T) {
	_, lanURL, _, _, _ := startGame(t)
	watch := post(t, lanURL+"api/watch", map[string]string{"name": "Observador"})
	token := watch["token"].(string)
	post(t, lanURL+"api/action", map[string]string{
		"token": token,
		"type":  "chat",
		"text":  "¡Buena jugada!",
	})

	resp, err := http.Get(lanURL + "api/state?token=" + token)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var state struct {
		Chat []ChatMsg `json:"chat"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if len(state.Chat) != 1 || state.Chat[0].From != "Observador" ||
		state.Chat[0].Text != "¡Buena jugada!" || state.Chat[0].Color != -1 {
		t.Fatalf("comentario de espectador inesperado: %+v", state.Chat)
	}
}

func TestClockExpiresAndAwardsOpponent(t *testing.T) {
	s := &Server{subs: make(map[*sub]struct{})}
	g := &Game{
		TimeMode:  "per-move",
		TimeLimit: 30 * time.Millisecond,
		Players:   [2]*Player{{Name: "Ana"}, {Name: "Luis"}},
	}
	s.mu.Lock()
	s.game = g
	s.startClockLocked(g)
	s.mu.Unlock()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		result, reason := g.Result, g.Reason
		s.mu.Unlock()
		if result != "" {
			if result != "0-1" || reason != "Tiempo agotado" {
				t.Fatalf("resultado por tiempo = %q (%q), se esperaba 0-1", result, reason)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("el reloj no terminó la partida")
}

func TestTotalClockConsumesOnlyPlayerWhoMoved(t *testing.T) {
	s := &Server{subs: make(map[*sub]struct{})}
	g := &Game{
		Players:     [2]*Player{{Name: "Ana", Token: "white"}, {Name: "Luis", Token: "black"}},
		TimeMode:    "total",
		TimeLimit:   10 * time.Second,
		Remaining:   [2]time.Duration{10 * time.Second, 10 * time.Second},
		TurnStarted: time.Now().Add(-4 * time.Second),
	}
	s.game = g

	req := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(
		`{"token":"white","type":"move","move":"e2e4","ply":0}`,
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("jugada con tiempo total: HTTP %d: %s", rec.Code, rec.Body.String())
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if g.Remaining[0] > 7*time.Second || g.Remaining[0] < 5*time.Second {
		t.Fatalf("reloj de blancas = %s; se esperaban cerca de 6 segundos", g.Remaining[0])
	}
	if g.Remaining[1] != 10*time.Second {
		t.Fatalf("el reloj de negras cambió antes de su turno: %s", g.Remaining[1])
	}
	if g.ClockTimer != nil {
		g.ClockTimer.Stop()
	}
}
