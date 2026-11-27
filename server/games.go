package server

import (
	"errors"
	"fmt"
	mrand "math/rand"
	"sort"
	"strings"
	"time"
)

type checkersState struct {
	Board     []int `json:"board"`
	Turn      int   `json:"turn"`
	ChainFrom int   `json:"chainFrom"`
}

type checkersMove struct {
	from     int
	to       int
	captured int
}

func newCheckersState() *checkersState {
	s := &checkersState{Board: make([]int, 64), ChainFrom: -1}
	for row := 0; row < 3; row++ {
		for col := 0; col < 8; col++ {
			if (row+col)%2 == 1 {
				s.Board[row*8+col] = -1
			}
		}
	}
	for row := 5; row < 8; row++ {
		for col := 0; col < 8; col++ {
			if (row+col)%2 == 1 {
				s.Board[row*8+col] = 1
			}
		}
	}
	return s
}

func checkersView(s *checkersState) *checkersState { return s }

func checkerMoves(s *checkersState, player int, fromOnly int) []checkersMove {
	captures := []checkersMove{}
	steps := []checkersMove{}
	dirs := [][2]int{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}}
	for from, piece := range s.Board {
		if piece == 0 || (player == 0 && piece < 0) || (player == 1 && piece > 0) ||
			(fromOnly >= 0 && from != fromOnly) {
			continue
		}
		row, col := from/8, from%8
		for _, d := range dirs {
			if abs(d[1]) == 1 && abs(piece) != 2 &&
				((player == 0 && d[1] > 0) || (player == 1 && d[1] < 0)) {
				continue
			}
			nr, nc := row+d[1], col+d[0]
			if nr < 0 || nr > 7 || nc < 0 || nc > 7 {
				continue
			}
			to := nr*8 + nc
			if s.Board[to] == 0 {
				steps = append(steps, checkersMove{from: from, to: to, captured: -1})
				continue
			}
			if (player == 0 && s.Board[to] > 0) || (player == 1 && s.Board[to] < 0) {
				continue
			}
			jr, jc := row+2*d[1], col+2*d[0]
			if jr >= 0 && jr < 8 && jc >= 0 && jc < 8 && s.Board[jr*8+jc] == 0 {
				captures = append(captures, checkersMove{from: from, to: jr*8 + jc, captured: to})
			}
		}
	}
	if len(captures) > 0 {
		return captures
	}
	if fromOnly >= 0 {
		return nil
	}
	return steps
}

func makeCheckersMove(g *Game, player, from, to int) error {
	s := g.Checkers
	if s == nil || player != s.Turn {
		return errors.New("no es tu turno")
	}
	if from < 0 || from >= 64 || to < 0 || to >= 64 {
		return errors.New("movimiento inválido")
	}
	moves := checkerMoves(s, player, s.ChainFrom)
	found := checkersMove{captured: -1}
	for _, m := range moves {
		if m.from == from && m.to == to {
			found = m
			break
		}
	}
	if found.from != from || found.to != to {
		return errors.New("movimiento ilegal; las capturas son obligatorias")
	}
	piece := s.Board[from]
	s.Board[from], s.Board[to] = 0, piece
	if found.captured >= 0 {
		s.Board[found.captured] = 0
	}
	if (player == 0 && to/8 == 0) || (player == 1 && to/8 == 7) {
		if piece > 0 {
			s.Board[to] = 2
		} else {
			s.Board[to] = -2
		}
	}
	if found.captured >= 0 && len(checkersCapturesFrom(s, player, to)) > 0 {
		s.ChainFrom = to
		return nil
	}
	s.ChainFrom = -1
	s.Turn = 1 - player
	hasPiece := false
	for _, p := range s.Board {
		if (s.Turn == 0 && p > 0) || (s.Turn == 1 && p < 0) {
			hasPiece = true
			break
		}
	}
	if !hasPiece || len(checkerMoves(s, s.Turn, -1)) == 0 {
		g.Result = fmt.Sprintf("winner:%d", player)
		g.Reason = "Victoria en Damas"
	}
	return nil
}

func checkersCapturesFrom(s *checkersState, player, from int) []checkersMove {
	moves := checkerMoves(s, player, from)
	out := make([]checkersMove, 0, len(moves))
	for _, m := range moves {
		if m.captured >= 0 {
			out = append(out, m)
		}
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

type card struct {
	ID    int    `json:"id"`
	Color string `json:"color"`
	Rank  string `json:"rank"`
}

type meld struct {
	Owner int    `json:"owner"`
	Cards []card `json:"cards"`
}

type cardGameState struct {
	Kind       string
	Hands      [][]card
	Deck       []card
	Discard    []card
	Melds      []meld
	Turn       int
	Direction  int
	Drawn      bool
	Opening    bool
	TurnCount  int
	HitAllowed bool
	UnoOwed    int
	Scores     []int
}

func cardDeck(uno bool) []card {
	out := []card{}
	id := 0
	colors := []string{"red", "yellow", "green", "blue"}
	for _, color := range colors {
		out = append(out, card{ID: id, Color: color, Rank: "0"})
		id++
		for _, rank := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "skip", "reverse", "draw2"} {
			for copy := 0; copy < 2; copy++ {
				out = append(out, card{ID: id, Color: color, Rank: rank})
				id++
			}
		}
	}
	if uno {
		for _, rank := range []string{"wild", "wild4"} {
			for copy := 0; copy < 4; copy++ {
				out = append(out, card{ID: id, Rank: rank})
				id++
			}
		}
	}
	mrand.New(mrand.NewSource(time.Now().UnixNano())).Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})
	return out
}

func newUnoState(players int) *cardGameState {
	s := &cardGameState{Kind: "uno", Hands: make([][]card, players), Direction: 1, UnoOwed: -1}
	s.Deck = cardDeck(true)
	for n := 0; n < 7; n++ {
		for p := 0; p < players; p++ {
			s.Hands[p] = append(s.Hands[p], s.takeDeckCard())
		}
	}
	for {
		top := s.takeDeckCard()
		if top.Rank == "skip" || top.Rank == "reverse" || top.Rank == "draw2" || top.Rank == "wild" || top.Rank == "wild4" {
			s.Deck = append(s.Deck, top) // al final, para no volver a sacarla
			continue
		}
		s.Discard = append(s.Discard, top)
		break
	}
	return s
}

func newGolpeadoState(players int) *cardGameState {
	s := &cardGameState{Kind: "golpeado", Hands: make([][]card, players), Direction: 1,
		Opening: true, HitAllowed: false, Scores: make([]int, players), UnoOwed: -1}
	decks := 1
	if players == 8 {
		decks = 2
	}
	s.Deck = golpeadoDeck(decks)
	for p := 0; p < players; p++ {
		count := 7
		if p == 0 {
			count = 8
		}
		for n := 0; n < count; n++ {
			s.Hands[p] = append(s.Hands[p], s.takeDeckCard())
		}
	}
	return s
}

func golpeadoDeck(decks int) []card {
	out := []card{}
	id := 0
	for deck := 0; deck < decks; deck++ {
		for _, suit := range []string{"hearts", "diamonds", "clubs", "spades"} {
			for _, rank := range []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"} {
				out = append(out, card{ID: id, Color: suit, Rank: rank})
				id++
			}
		}
	}
	mrand.New(mrand.NewSource(time.Now().UnixNano())).Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})
	return out
}

func (s *cardGameState) takeDeckCard() card {
	if len(s.Deck) == 0 && len(s.Discard) > 1 {
		top := s.Discard[len(s.Discard)-1]
		s.Deck = append([]card(nil), s.Discard[:len(s.Discard)-1]...)
		s.Discard = []card{top}
		mrand.New(mrand.NewSource(time.Now().UnixNano())).Shuffle(len(s.Deck), func(i, j int) {
			s.Deck[i], s.Deck[j] = s.Deck[j], s.Deck[i]
		})
	}
	if len(s.Deck) == 0 {
		return card{ID: -1}
	}
	c := s.Deck[0]
	s.Deck = s.Deck[1:]
	return c
}

func cardGameView(s *cardGameState, player int) map[string]any {
	counts := make([]int, len(s.Hands))
	for i := range s.Hands {
		counts[i] = len(s.Hands[i])
	}
	view := map[string]any{
		"kind": s.Kind, "turn": s.Turn, "direction": s.Direction, "counts": counts,
		"discard": s.Discard, "drawn": s.Drawn, "opening": s.Opening,
		"turnCount": s.TurnCount, "hitAllowed": s.HitAllowed, "melds": s.Melds,
		"scores": s.Scores, "unoOwed": s.UnoOwed,
	}
	if player >= 0 && player < len(s.Hands) {
		view["hand"] = s.Hands[player]
	} else {
		view["hand"] = []card{}
	}
	return view
}

func makeCardAction(g *Game, player int, action string, cardID int, color string, ids []int, meldIndex int, callUno bool) error {
	s := g.Cards
	if s == nil || player != s.Turn {
		return errors.New("no es tu turno")
	}
	if s.Kind == "uno" {
		return makeUnoAction(g, s, player, action, cardID, color, callUno)
	}
	return makeGolpeadoAction(g, s, player, action, cardID, ids, meldIndex)
}

func makeUnoAction(g *Game, s *cardGameState, player int, action string, cardID int, color string, callUno bool) error {
	hand := s.Hands[player]
	if s.UnoOwed >= 0 && action == "catch" && s.UnoOwed != player {
		for i := 0; i < 2; i++ {
			c := s.takeDeckCard()
			if c.ID >= 0 {
				s.Hands[s.UnoOwed] = append(s.Hands[s.UnoOwed], c)
			}
		}
		s.UnoOwed = -1
		return nil
	}
	switch action {
	case "draw":
		if s.Drawn {
			return errors.New("ya robaste en este turno")
		}
		top := s.Discard[len(s.Discard)-1]
		if handHasPlayable(hand, top) {
			return errors.New("tienes una carta que puedes jugar")
		}
		c := s.takeDeckCard()
		if c.ID < 0 {
			return errors.New("no quedan cartas para robar")
		}
		if s.UnoOwed >= 0 && player != s.UnoOwed {
			s.UnoOwed = -1
		}
		s.Hands[player] = append(hand, c)
		s.Drawn = true
		if !unoPlayable(c, s.Discard[len(s.Discard)-1]) {
			advanceCardTurn(s)
		}
	case "play":
		idx := findCard(hand, cardID)
		if idx < 0 {
			return errors.New("esa carta no está en tu mano")
		}
		c := hand[idx]
		top := s.Discard[len(s.Discard)-1]
		if !unoPlayable(c, top) {
			return errors.New("esa carta no se puede jugar ahora")
		}
		if c.Rank == "wild" || c.Rank == "wild4" {
			if color != "red" && color != "yellow" && color != "green" && color != "blue" {
				return errors.New("elige un color")
			}
			if c.Rank == "wild4" {
				for i, held := range hand {
					if i != idx && held.Color == top.Color {
						return errors.New("no puedes usar +4 si tienes una carta del color actual")
					}
				}
			}
			c.Color = color
		}
		if s.UnoOwed >= 0 && player != s.UnoOwed {
			s.UnoOwed = -1
		}
		s.Hands[player] = removeCard(hand, idx)
		s.Discard = append(s.Discard, c)
		if len(s.Hands[player]) == 0 {
			g.Result = fmt.Sprintf("winner:%d", player)
			g.Reason = "Ganó UNO"
			return nil
		}
		if len(s.Hands[player]) == 1 && !callUno {
			s.UnoOwed = player
		}
		switch c.Rank {
		case "reverse":
			s.Direction *= -1
			advanceCardTurn(s)
			if len(s.Hands) == 2 {
				advanceCardTurn(s)
			}
		case "skip":
			advanceCardTurn(s)
			advanceCardTurn(s)
		case "draw2":
			next := nextCardPlayer(s, player)
			drawCards(s, next, 2)
			advanceCardTurn(s)
			advanceCardTurn(s)
		case "wild4":
			next := nextCardPlayer(s, player)
			drawCards(s, next, 4)
			advanceCardTurn(s)
			advanceCardTurn(s)
		default:
			advanceCardTurn(s)
		}
		if s.UnoOwed >= 0 && s.Turn == s.UnoOwed {
			s.UnoOwed = -1
		}
	default:
		return errors.New("acción de UNO desconocida")
	}
	return nil
}

func handHasPlayable(hand []card, top card) bool {
	for _, c := range hand {
		if unoPlayable(c, top) {
			return true
		}
	}
	return false
}

func unoPlayable(c, top card) bool {
	return c.Rank == "wild" || c.Rank == "wild4" || c.Color == top.Color || c.Rank == top.Rank
}

func makeGolpeadoAction(g *Game, s *cardGameState, player int, action string, cardID int, ids []int, meldIndex int) error {
	switch action {
	case "draw":
		if s.Drawn || (s.Opening && player == 0) {
			return errors.New("no puedes robar ahora")
		}
		c := s.takeDeckCard()
		if c.ID < 0 {
			return errors.New("no quedan cartas para robar")
		}
		s.Hands[player] = append(s.Hands[player], c)
		s.Drawn = true
	case "discard":
		if s.Opening && player == 0 {
			idx := findCard(s.Hands[player], cardID)
			if idx < 0 {
				return errors.New("esa carta no está en tu mano")
			}
			s.Discard = append(s.Discard, s.Hands[player][idx])
			s.Hands[player] = removeCard(s.Hands[player], idx)
			s.Opening = false
			advanceGolpeadoTurn(s)
			return nil
		}
		if !s.Drawn {
			return errors.New("roba una carta antes de descartar")
		}
		idx := findCard(s.Hands[player], cardID)
		if idx < 0 {
			return errors.New("esa carta no está en tu mano")
		}
		s.Discard = append(s.Discard, s.Hands[player][idx])
		s.Hands[player] = removeCard(s.Hands[player], idx)
		advanceGolpeadoTurn(s)
	case "meld":
		if !s.Drawn || len(ids) < 3 {
			return errors.New("roba primero y selecciona al menos tres cartas")
		}
		cards, rest, ok := selectCards(s.Hands[player], ids)
		if !ok || !validMeld(cards) {
			return errors.New("selecciona una tercia o una escalera válida")
		}
		if len(rest) == 0 {
			return errors.New("deja al menos una carta para descartar")
		}
		s.Hands[player] = rest
		s.Melds = append(s.Melds, meld{Owner: player, Cards: cards})
	case "attach":
		if !s.Drawn || meldIndex < 0 || meldIndex >= len(s.Melds) || len(ids) == 0 {
			return errors.New("selecciona una combinación y cartas para enchufar")
		}
		cards, rest, ok := selectCards(s.Hands[player], ids)
		if !ok {
			return errors.New("una carta seleccionada no está en tu mano")
		}
		combined := append(append([]card(nil), s.Melds[meldIndex].Cards...), cards...)
		if !validMeld(combined) {
			return errors.New("esas cartas no completan la combinación")
		}
		if len(rest) == 0 {
			return errors.New("deja al menos una carta para descartar")
		}
		s.Hands[player] = rest
		s.Melds[meldIndex].Cards = combined
	case "hit":
		if !s.HitAllowed || s.Drawn {
			return errors.New("solo puedes golpear al inicio de tu turno después de la primera vuelta")
		}
		scores := make([]int, len(s.Hands))
		for i := range s.Hands {
			for _, c := range s.Hands[i] {
				scores[i] += golpeadoPoints(c)
			}
		}
		minOther := -1
		for i, score := range scores {
			if i != player && (minOther < 0 || score < minOther) {
				minOther = score
			}
		}
		if scores[player] >= minOther {
			scores[player] *= 2
		}
		winner := 0
		tie := false
		for i := 1; i < len(scores); i++ {
			if scores[i] < scores[winner] {
				winner, tie = i, false
			} else if scores[i] == scores[winner] {
				tie = true
			}
		}
		g.Cards.Scores = scores
		if tie {
			g.Result = "1/2-1/2"
		} else {
			g.Result = fmt.Sprintf("winner:%d", winner)
		}
		g.Reason = "Golpeó: puntajes " + formatScores(scores)
	default:
		return errors.New("acción de Golpeado desconocida")
	}
	return nil
}

// autoCardTurn juega por el jugador al que se le agotó el tiempo: en UNO roba
// una carta (si aún no robó) y pasa; en Golpeado roba si hace falta y descarta
// la última carta de su mano.
func autoCardTurn(g *Game) {
	s := g.Cards
	p := s.Turn
	if s.Kind == "uno" {
		if !s.Drawn {
			drawCards(s, p, 1)
		}
		if s.UnoOwed >= 0 && p != s.UnoOwed {
			s.UnoOwed = -1
		}
		advanceCardTurn(s)
		if s.UnoOwed >= 0 && s.Turn == s.UnoOwed {
			s.UnoOwed = -1
		}
		return
	}
	if !(s.Opening && p == 0) && !s.Drawn {
		drawCards(s, p, 1)
	}
	if n := len(s.Hands[p]); n > 0 {
		s.Discard = append(s.Discard, s.Hands[p][n-1])
		s.Hands[p] = s.Hands[p][:n-1]
	}
	s.Opening = false
	advanceCardTurn(s)
}

func advanceCardTurn(s *cardGameState) {
	s.Turn = nextCardPlayer(s, s.Turn)
	s.Drawn = false
	s.TurnCount++
	if s.Kind == "golpeado" && s.TurnCount >= len(s.Hands) {
		s.HitAllowed = true
	}
}

func advanceGolpeadoTurn(s *cardGameState) { advanceCardTurn(s) }

func nextCardPlayer(s *cardGameState, from int) int {
	return (from + s.Direction + len(s.Hands)) % len(s.Hands)
}

func drawCards(s *cardGameState, player, count int) {
	for i := 0; i < count; i++ {
		c := s.takeDeckCard()
		if c.ID >= 0 {
			s.Hands[player] = append(s.Hands[player], c)
		}
	}
}

func findCard(hand []card, id int) int {
	for i, c := range hand {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func removeCard(hand []card, idx int) []card {
	return append(hand[:idx], hand[idx+1:]...)
}

func selectCards(hand []card, ids []int) ([]card, []card, bool) {
	seen := map[int]bool{}
	selected := []card{}
	rest := append([]card(nil), hand...)
	for _, id := range ids {
		if seen[id] {
			return nil, nil, false
		}
		seen[id] = true
		idx := findCard(rest, id)
		if idx < 0 {
			return nil, nil, false
		}
		selected = append(selected, rest[idx])
		rest = removeCard(rest, idx)
	}
	return selected, rest, true
}

func validMeld(cards []card) bool {
	if len(cards) < 3 {
		return false
	}
	group := true
	for _, c := range cards[1:] {
		if c.Rank != cards[0].Rank {
			group = false
			break
		}
	}
	if group {
		return true
	}
	for _, c := range cards {
		if c.Color != cards[0].Color {
			return false
		}
	}
	ranks := make([]int, len(cards))
	for i, c := range cards {
		n, ok := golpeadoRank(c.Rank)
		if !ok {
			return false
		}
		ranks[i] = n
	}
	sort.Ints(ranks)
	for i := 1; i < len(ranks); i++ {
		if ranks[i] != ranks[i-1]+1 {
			return false
		}
	}
	return true
}

func golpeadoRank(rank string) (int, bool) {
	switch rank {
	case "A":
		return 1, true
	case "J":
		return 11, true
	case "Q":
		return 12, true
	case "K":
		return 13, true
	}
	var n int
	_, err := fmt.Sscanf(rank, "%d", &n)
	return n, err == nil && n >= 2 && n <= 10
}

func golpeadoPoints(c card) int {
	if c.Rank == "A" {
		return 1
	}
	if c.Rank == "J" || c.Rank == "Q" || c.Rank == "K" {
		return 10
	}
	var n int
	_, _ = fmt.Sscanf(c.Rank, "%d", &n)
	return n
}

func formatScores(scores []int) string {
	parts := make([]string, len(scores))
	for i, score := range scores {
		parts[i] = fmt.Sprintf("J%d: %d", i+1, score)
	}
	return strings.Join(parts, " · ")
}
