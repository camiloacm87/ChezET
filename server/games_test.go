package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestCheckersRequiresCaptureAndDeclaresWinner(t *testing.T) {
	s := &checkersState{Board: make([]int, 64), Turn: 0, ChainFrom: -1}
	s.Board[40], s.Board[33], s.Board[44] = 1, -1, 1
	g := &Game{GameType: "checkers", Checkers: s}
	if err := makeCheckersMove(g, 0, 44, 35); err == nil {
		t.Fatal("se permitió una jugada normal cuando había una captura")
	}
	if err := makeCheckersMove(g, 0, 40, 26); err != nil {
		t.Fatalf("captura obligatoria rechazada: %v", err)
	}
	if g.Result != "winner:0" {
		t.Fatalf("resultado tras capturar la última ficha: %q", g.Result)
	}
}

func TestGolpeadoDealsAndHidesOpponentsHands(t *testing.T) {
	for _, players := range []int{2, 4, 8} {
		s := newGolpeadoState(players)
		if len(s.Hands[0]) != 8 {
			t.Fatalf("%d jugadores: mano inicial del repartidor = %d, esperadas 8", players, len(s.Hands[0]))
		}
		for i := 1; i < players; i++ {
			if len(s.Hands[i]) != 7 {
				t.Fatalf("%d jugadores: mano inicial de jugador %d = %d, esperadas 7", players, i, len(s.Hands[i]))
			}
		}
		view := cardGameView(s, 1)
		if len(view["hand"].([]card)) != 7 {
			t.Fatalf("%d jugadores: mano privada incorrecta", players)
		}
		if _, ok := view["deck"]; ok {
			t.Fatal("se expuso el mazo a los jugadores")
		}
	}
}

func TestGolpeadoHitDoublesStrikerIfNotLowest(t *testing.T) {
	s := &cardGameState{
		Kind: "golpeado", Turn: 0, HitAllowed: true,
		Hands: [][]card{
			{{ID: 1, Rank: "5"}},
			{{ID: 2, Rank: "2"}},
		},
	}
	g := &Game{Cards: s}
	if err := makeCardAction(g, 0, "hit", 0, "", nil, -1, false); err != nil {
		t.Fatalf("golpeo rechazado: %v", err)
	}
	if g.Result != "winner:1" || s.Scores[0] != 10 || s.Scores[1] != 2 {
		t.Fatalf("resultado/puntajes inesperados: result=%q scores=%v", g.Result, s.Scores)
	}
}

func TestUnoMissedCallCanBeCaught(t *testing.T) {
	s := &cardGameState{
		Kind: "uno", Turn: 0, Direction: 1, UnoOwed: -1,
		Hands: [][]card{
			{{ID: 1, Color: "red", Rank: "5"}, {ID: 2, Color: "blue", Rank: "4"}},
			{{ID: 3, Color: "green", Rank: "6"}},
		},
		Discard: []card{{ID: 4, Color: "yellow", Rank: "5"}},
		Deck:    []card{{ID: 5, Color: "red", Rank: "2"}, {ID: 6, Color: "blue", Rank: "8"}},
	}
	g := &Game{Cards: s}
	if err := makeCardAction(g, 0, "play", 1, "", nil, -1, false); err != nil {
		t.Fatalf("jugada legal rechazada: %v", err)
	}
	if s.UnoOwed != 0 || s.Turn != 1 {
		t.Fatalf("no se registró la oportunidad de reclamar UNO: owed=%d turn=%d", s.UnoOwed, s.Turn)
	}
	if err := makeCardAction(g, 1, "catch", 0, "", nil, -1, false); err != nil {
		t.Fatalf("reclamo de UNO rechazado: %v", err)
	}
	if len(s.Hands[0]) != 3 || s.UnoOwed != -1 || s.Turn != 1 {
		t.Fatalf("penalización incorrecta: mano=%d owed=%d turn=%d", len(s.Hands[0]), s.UnoOwed, s.Turn)
	}
}

func TestCardGameLobbyStartsWithTwoOrMorePlayers(t *testing.T) {
	lan := freePort(t)
	local, err := Start(lan)
	if err != nil {
		t.Fatal(err)
	}
	hostTok := post(t, local+"api/create", map[string]any{"name": "Ana", "gameType": "uno"})["token"].(string)
	lanURL := fmt.Sprintf("http://127.0.0.1:%d/", lan)
	current.mu.Lock()
	if current.game.Started || len(current.game.ExtraPlayers) != 1 {
		t.Fatal("la sala debe empezar con el anfitrión y sin iniciar")
	}
	current.mu.Unlock()
	for _, name := range []string{"Luis", "Eva"} {
		post(t, lanURL+"api/join", map[string]string{"name": name})
	}
	b, _ := json.Marshal(map[string]string{"token": hostTok, "type": "start"})
	resp, err := http.Post(local+"api/action", "application/json", bytes.NewReader(b))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("iniciar con 3 jugadores falló: %v %v", err, resp)
	}
	current.mu.Lock()
	ok := current.game.Started && len(current.game.Cards.Hands) == 3
	current.mu.Unlock()
	if !ok {
		t.Fatal("la partida debe iniciar con 3 manos")
	}
	Shutdown()
}

func TestAutoCardTurnOnTimeout(t *testing.T) {
	g := &Game{Cards: newUnoState(3)}
	before := len(g.Cards.Hands[0])
	autoCardTurn(g)
	if g.Cards.Turn != 1 || len(g.Cards.Hands[0]) != before+1 {
		t.Fatalf("UNO: turno=%d cartas=%d", g.Cards.Turn, len(g.Cards.Hands[0]))
	}
	g = &Game{Cards: newGolpeadoState(3)}
	autoCardTurn(g) // el jugador 0 descarta para abrir
	if g.Cards.Turn != 1 || g.Cards.Opening || len(g.Cards.Hands[0]) != 7 {
		t.Fatalf("Golpeado apertura: turno=%d cartas=%d", g.Cards.Turn, len(g.Cards.Hands[0]))
	}
	autoCardTurn(g) // el jugador 1 roba y descarta
	if g.Cards.Turn != 2 || len(g.Cards.Hands[1]) != 7 {
		t.Fatalf("Golpeado turno: turno=%d cartas=%d", g.Cards.Turn, len(g.Cards.Hands[1]))
	}
}

func TestCheckersClockExpiryAwardsOpponent(t *testing.T) {
	s := &Server{}
	g := &Game{GameType: "checkers", Checkers: newCheckersState(), TimeMode: "per-move",
		TimeLimit: time.Second, Remaining: [2]time.Duration{time.Second, time.Second},
		TurnStarted: time.Now().Add(-2 * time.Second)}
	g.Checkers.Turn = 1
	if !s.clockExpiredLocked(g) || g.Result != "winner:0" || g.Reason != "Tiempo agotado" {
		t.Fatalf("resultado = %q (%s)", g.Result, g.Reason)
	}
}
