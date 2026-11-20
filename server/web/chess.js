// Motor de reglas de ajedrez. Casillas 0..63: a1 = 0, h1 = 7, a8 = 56, h8 = 63.
// Piezas: mayúsculas = blancas (PNBRQK), minúsculas = negras.
const Chess = (() => {
  const FILES = 'abcdefgh';
  const sqName = s => FILES[s & 7] + ((s >> 3) + 1);
  const colorOf = p => (p ? (p === p.toUpperCase() ? 'w' : 'b') : null);
  const other = c => (c === 'w' ? 'b' : 'w');

  const KN = [[1, 2], [2, 1], [2, -1], [1, -2], [-1, -2], [-2, -1], [-2, 1], [-1, 2]];
  const KG = [[1, 0], [1, 1], [0, 1], [-1, 1], [-1, 0], [-1, -1], [0, -1], [1, -1]];
  const DIAG = [[1, 1], [1, -1], [-1, 1], [-1, -1]];
  const ORTH = [[1, 0], [-1, 0], [0, 1], [0, -1]];

  function step(s, df, dr) {
    const f = (s & 7) + df, r = (s >> 3) + dr;
    return f < 0 || f > 7 || r < 0 || r > 7 ? -1 : r * 8 + f;
  }

  function initial() {
    const b = new Array(64).fill(null);
    const back = 'RNBQKBNR';
    for (let f = 0; f < 8; f++) {
      b[f] = back[f];
      b[8 + f] = 'P';
      b[48 + f] = 'p';
      b[56 + f] = back[f].toLowerCase();
    }
    return { board: b, turn: 'w', castle: { K: true, Q: true, k: true, q: true }, ep: -1, half: 0, full: 1 };
  }

  const clone = s => ({ board: s.board.slice(), turn: s.turn, castle: { ...s.castle }, ep: s.ep, half: s.half, full: s.full });

  // ¿La casilla sq está atacada por el bando "by"?
  function attacked(board, sq, by) {
    const w = by === 'w';
    const P = w ? 'P' : 'p', N = w ? 'N' : 'n', B = w ? 'B' : 'b', R = w ? 'R' : 'r', Q = w ? 'Q' : 'q', K = w ? 'K' : 'k';
    const pr = w ? -1 : 1; // un peón blanco ataca hacia arriba, así que está una fila por debajo
    for (const df of [-1, 1]) {
      const t = step(sq, df, pr);
      if (t >= 0 && board[t] === P) return true;
    }
    for (const [df, dr] of KN) {
      const t = step(sq, df, dr);
      if (t >= 0 && board[t] === N) return true;
    }
    for (const [df, dr] of KG) {
      const t = step(sq, df, dr);
      if (t >= 0 && board[t] === K) return true;
    }
    for (const [dirs, a, c] of [[DIAG, B, Q], [ORTH, R, Q]]) {
      for (const [df, dr] of dirs) {
        let t = sq;
        while ((t = step(t, df, dr)) >= 0) {
          const p = board[t];
          if (p) {
            if (p === a || p === c) return true;
            break;
          }
        }
      }
    }
    return false;
  }

  function pseudo(s) {
    const out = [], b = s.board, me = s.turn, opp = other(me);
    const add = (from, to, extra) => out.push({ from, to, ...extra });
    for (let sq = 0; sq < 64; sq++) {
      const p = b[sq];
      if (!p || colorOf(p) !== me) continue;
      const t = p.toUpperCase();
      if (t === 'P') {
        const dir = me === 'w' ? 1 : -1, startR = me === 'w' ? 1 : 6, lastR = me === 'w' ? 7 : 0;
        const pawnTo = to => {
          if (to >> 3 === lastR) for (const pr of 'qrbn') add(sq, to, { promo: pr });
          else add(sq, to, {});
        };
        const one = step(sq, 0, dir);
        if (one >= 0 && !b[one]) {
          pawnTo(one);
          const two = step(sq, 0, 2 * dir);
          if (sq >> 3 === startR && !b[two]) add(sq, two, { double: true });
        }
        for (const df of [-1, 1]) {
          const c = step(sq, df, dir);
          if (c < 0) continue;
          if (b[c] && colorOf(b[c]) === opp) pawnTo(c);
          else if (c === s.ep && !b[c]) add(sq, c, { ep: true });
        }
      } else if (t === 'N' || t === 'K') {
        for (const [df, dr] of t === 'N' ? KN : KG) {
          const c = step(sq, df, dr);
          if (c >= 0 && colorOf(b[c]) !== me) add(sq, c, {});
        }
        if (t === 'K') {
          const base = me === 'w' ? 0 : 56, R = me === 'w' ? 'R' : 'r';
          const ck = me === 'w' ? 'K' : 'k', cq = me === 'w' ? 'Q' : 'q';
          if (sq === base + 4 && !attacked(b, sq, opp)) {
            if (s.castle[ck] && !b[base + 5] && !b[base + 6] && b[base + 7] === R &&
                !attacked(b, base + 5, opp) && !attacked(b, base + 6, opp)) add(sq, base + 6, { castle: 'K' });
            if (s.castle[cq] && !b[base + 1] && !b[base + 2] && !b[base + 3] && b[base] === R &&
                !attacked(b, base + 3, opp) && !attacked(b, base + 2, opp)) add(sq, base + 2, { castle: 'Q' });
          }
        }
      } else {
        const dirs = t === 'B' ? DIAG : t === 'R' ? ORTH : DIAG.concat(ORTH);
        for (const [df, dr] of dirs) {
          let c = sq;
          while ((c = step(c, df, dr)) >= 0) {
            if (b[c]) {
              if (colorOf(b[c]) !== me) add(sq, c, {});
              break;
            }
            add(sq, c, {});
          }
        }
      }
    }
    return out;
  }

  function apply(s, m) {
    const n = clone(s), b = n.board, p = b[m.from], me = s.turn;
    const captured = m.ep ? (me === 'w' ? 'p' : 'P') : b[m.to];
    b[m.to] = m.promo ? (me === 'w' ? m.promo.toUpperCase() : m.promo) : p;
    b[m.from] = null;
    if (m.ep) b[m.to + (me === 'w' ? -8 : 8)] = null;
    if (m.castle) {
      const base = me === 'w' ? 0 : 56;
      if (m.castle === 'K') { b[base + 5] = b[base + 7]; b[base + 7] = null; }
      else { b[base + 3] = b[base]; b[base] = null; }
    }
    n.ep = m.double ? (m.from + m.to) / 2 : -1;
    for (const sq of [m.from, m.to]) {
      if (sq === 4) n.castle.K = n.castle.Q = false;
      if (sq === 60) n.castle.k = n.castle.q = false;
      if (sq === 0) n.castle.Q = false;
      if (sq === 7) n.castle.K = false;
      if (sq === 56) n.castle.q = false;
      if (sq === 63) n.castle.k = false;
    }
    n.half = p.toUpperCase() === 'P' || captured ? 0 : s.half + 1;
    if (me === 'b') n.full++;
    n.turn = other(me);
    return n;
  }

  const kingSq = (b, c) => b.indexOf(c === 'w' ? 'K' : 'k');
  const inCheck = s => attacked(s.board, kingSq(s.board, s.turn), other(s.turn));

  function legal(s) {
    return pseudo(s).filter(m => {
      const n = apply(s, m);
      return !attacked(n.board, kingSq(n.board, s.turn), n.turn);
    });
  }

  const uci = m => sqName(m.from) + sqName(m.to) + (m.promo || '');

  function san(s, m, all) {
    let str;
    const piece = s.board[m.from], p = piece.toUpperCase();
    if (m.castle) str = m.castle === 'K' ? 'O-O' : 'O-O-O';
    else {
      const cap = s.board[m.to] || m.ep;
      if (p === 'P') {
        str = (cap ? FILES[m.from & 7] + 'x' : '') + sqName(m.to) + (m.promo ? '=' + m.promo.toUpperCase() : '');
      } else {
        const others = all.filter(o => o.to === m.to && o.from !== m.from && s.board[o.from] === piece);
        let dis = '';
        if (others.length) {
          const sameFile = others.some(o => (o.from & 7) === (m.from & 7));
          const sameRank = others.some(o => o.from >> 3 === m.from >> 3);
          dis = !sameFile ? FILES[m.from & 7] : !sameRank ? String((m.from >> 3) + 1) : sqName(m.from);
        }
        str = p + dis + (cap ? 'x' : '') + sqName(m.to);
      }
    }
    const n = apply(s, m);
    if (inCheck(n)) str += legal(n).length ? '+' : '#';
    return str;
  }

  function posKey(s) {
    let ep = '-';
    if (s.ep >= 0) {
      const P = s.turn === 'w' ? 'P' : 'p', dr = s.turn === 'w' ? -1 : 1;
      for (const df of [-1, 1]) { const t = step(s.ep, df, dr); if (t >= 0 && s.board[t] === P) ep = s.ep; }
    }
    const c = s.castle;
    return s.board.map(p => p || '.').join('') + s.turn + (c.K ? 'K' : '') + (c.Q ? 'Q' : '') + (c.k ? 'k' : '') + (c.q ? 'q' : '') + ep;
  }

  function insufficient(b) {
    const minors = [];
    for (let sq = 0; sq < 64; sq++) {
      const p = b[sq];
      if (!p || p.toUpperCase() === 'K') continue;
      const t = p.toUpperCase();
      if (t === 'P' || t === 'R' || t === 'Q') return false;
      minors.push({ t, sq });
    }
    if (minors.length <= 1) return true;
    // Solo alfiles, todos en casillas del mismo color
    if (minors.every(m => m.t === 'B')) {
      const col = sq => ((sq >> 3) + (sq & 7)) % 2;
      return minors.every(m => col(m.sq) === col(minors[0].sq));
    }
    return false;
  }

  // Reconstruye la partida a partir de la lista de jugadas UCI.
  function replay(ucis) {
    let s = initial();
    const hist = [];
    const keys = [posKey(s)];
    for (const u of ucis) {
      const ms = legal(s);
      const m = ms.find(x => uci(x) === u);
      if (!m) throw new Error('Jugada ilegal en el historial: ' + u);
      hist.push({ san: san(s, m, ms), from: m.from, to: m.to, uci: u, capture: !!(s.board[m.to] || m.ep), color: s.turn });
      s = apply(s, m);
      keys.push(posKey(s));
    }
    const moves = legal(s);
    const check = inCheck(s);
    let over = null;
    if (!moves.length) {
      over = check
        ? { result: s.turn === 'w' ? '0-1' : '1-0', reason: 'Jaque mate' }
        : { result: '1/2-1/2', reason: 'Rey ahogado' };
    } else if (insufficient(s.board)) over = { result: '1/2-1/2', reason: 'Material insuficiente' };
    else if (s.half >= 100) over = { result: '1/2-1/2', reason: 'Regla de los 50 movimientos' };
    else if (keys.filter(k => k === keys[keys.length - 1]).length >= 3) over = { result: '1/2-1/2', reason: 'Triple repetición' };
    return { state: s, hist, legal: moves, check, over };
  }

  return { replay, uci, colorOf, sqName, initial, legal, apply };
})();

if (typeof module !== 'undefined') module.exports = Chess;
